#!/usr/bin/env python3
"""Ingest LoCoMo conversations into a fresh Grasshopper database.

Uses raw 3-turn windows (Basic RAG write strategy from Yuan et al.) —
no extraction, no summarization. Each window becomes a memory entry.

Writes directly to SQLite for speed (~2000 inserts in seconds vs minutes via CLI).

Usage:
    python eval/ingest.py [--db eval/data/eval.db] [--data eval/data/locomo10.json]
"""

import argparse
import hashlib
import json
import sqlite3
import sys
from datetime import datetime, timezone
from pathlib import Path


def download_locomo(dest: Path):
    """Download locomo10.json from GitHub if not present."""
    if dest.exists():
        print(f"Data already exists: {dest}")
        return
    url = "https://github.com/snap-research/locomo/raw/refs/heads/main/data/locomo10.json"
    print(f"Downloading locomo10.json from {url}...")
    import urllib.request
    dest.parent.mkdir(parents=True, exist_ok=True)
    urllib.request.urlretrieve(url, dest)
    print(f"Downloaded to {dest} ({dest.stat().st_size:,} bytes)")


def init_db(db_path: Path) -> sqlite3.Connection:
    """Open Grasshopper DB and create schema if needed.
    Mirrors store.rs schema creation."""
    conn = sqlite3.connect(str(db_path))
    conn.execute("PRAGMA journal_mode = WAL")
    conn.execute("PRAGMA synchronous = NORMAL")
    conn.execute("PRAGMA foreign_keys = ON")

    # Create the chunks table (simplified — only fields we need for memory)
    conn.executescript("""
        CREATE TABLE IF NOT EXISTS codebases (
            id INTEGER PRIMARY KEY, dir_path TEXT UNIQUE NOT NULL, name TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS indexed_files (
            id INTEGER PRIMARY KEY, codebase_id INTEGER NOT NULL REFERENCES codebases(id),
            file_path TEXT NOT NULL, file_hash TEXT NOT NULL, indexed_at TEXT NOT NULL,
            UNIQUE(codebase_id, file_path)
        );
        CREATE TABLE IF NOT EXISTS chunks (
            id INTEGER PRIMARY KEY,
            kind TEXT NOT NULL DEFAULT 'code',
            title TEXT NOT NULL DEFAULT '',
            content TEXT NOT NULL DEFAULT '',
            snippet TEXT NOT NULL DEFAULT '',
            symbol_name TEXT,
            symbol_kind TEXT,
            signature TEXT,
            file_path TEXT,
            language TEXT,
            start_line INTEGER,
            end_line INTEGER,
            codebase_id INTEGER REFERENCES codebases(id),
            chunk_key TEXT,
            file_hash TEXT,
            content_hash TEXT,
            embedding BLOB,
            embedding_model TEXT,
            memory_type TEXT,
            descriptors TEXT DEFAULT '',
            source TEXT,
            access_count INTEGER NOT NULL DEFAULT 0,
            last_accessed TEXT,
            salience REAL NOT NULL DEFAULT 0.5,
            archived INTEGER NOT NULL DEFAULT 0,
            agent_id TEXT,
            created_at TEXT NOT NULL DEFAULT (datetime('now')),
            updated_at TEXT NOT NULL DEFAULT (datetime('now')),
            UNIQUE(chunk_key)
        );
        CREATE TABLE IF NOT EXISTS graph (
            id INTEGER PRIMARY KEY, codebase_id INTEGER REFERENCES codebases(id),
            file_path TEXT NOT NULL, symbol TEXT NOT NULL,
            role TEXT NOT NULL, kind TEXT NOT NULL, line INTEGER NOT NULL,
            UNIQUE(codebase_id, file_path, symbol, role, kind, line)
        );
        CREATE TABLE IF NOT EXISTS handoffs (
            id INTEGER PRIMARY KEY, project TEXT NOT NULL, summary TEXT NOT NULL,
            next_steps TEXT NOT NULL DEFAULT '', context TEXT NOT NULL DEFAULT '',
            agent_id TEXT, created_at TEXT NOT NULL DEFAULT (datetime('now'))
        );
    """)

    # Create FTS5 index if not exists
    try:
        conn.execute("""
            CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
                title, content, snippet, symbol_name, descriptors,
                content='chunks', content_rowid='id',
                tokenize='unicode61 remove_diacritics 2'
            )
        """)
    except sqlite3.OperationalError:
        pass  # Already exists

    return conn


def extract_sessions(conversation: dict) -> list[dict]:
    """Extract ordered sessions from a conversation dict."""
    sessions = []
    i = 1  # Sessions are 1-indexed in locomo10.json
    while True:
        key = f"session_{i}"
        dt_key = f"session_{i}_date_time"
        if key not in conversation:
            break
        turns = conversation[key]
        datetime_str = conversation.get(dt_key, "")
        sessions.append({
            "index": i,
            "datetime": datetime_str,
            "turns": turns,
        })
        i += 1
    return sessions


def window_turns(sessions: list[dict], window_size: int = 3) -> list[str]:
    """Create raw 3-turn windows across all sessions."""
    all_turns = []
    for session in sessions:
        dt = session["datetime"]
        for turn in session["turns"]:
            speaker = turn.get("speaker", "?")
            text = turn.get("text", "")
            dia_id = turn.get("dia_id", "")
            all_turns.append(f"[{dia_id}] [{dt}] {speaker}: {text}")

    windows = []
    for i in range(0, len(all_turns), window_size):
        chunk = all_turns[i : i + window_size]
        windows.append("\n".join(chunk))
    return windows


def insert_memory(conn: sqlite3.Connection, title: str, content: str, memory_type: str = "episode"):
    """Insert a memory entry directly into the database."""
    now = datetime.now(timezone.utc).isoformat()
    content_hash = hashlib.sha256(content.encode()).hexdigest()[:16]

    cursor = conn.execute(
        """INSERT INTO chunks (kind, title, content, memory_type, descriptors, salience,
                               content_hash, agent_id, created_at, updated_at)
           VALUES ('memory', ?, ?, ?, '', 0.5, ?, 'eval', ?, ?)""",
        (title, content, memory_type, content_hash, now, now),
    )
    chunk_id = cursor.lastrowid

    conn.execute(
        """INSERT INTO chunks_fts (rowid, title, content, snippet, symbol_name, descriptors)
           VALUES (?, ?, ?, '', '', '')""",
        (chunk_id, title, content),
    )
    return chunk_id


def main():
    parser = argparse.ArgumentParser(description="Ingest LoCoMo into Grasshopper")
    parser.add_argument("--db", default="eval/data/eval.db", help="Database path")
    parser.add_argument("--data", default="eval/data/locomo10.json", help="LoCoMo data file")
    parser.add_argument("--window", type=int, default=3, help="Turn window size")
    parser.add_argument("--download", action="store_true", help="Download data if missing")
    args = parser.parse_args()

    data_path = Path(args.data)
    db_path = Path(args.db)

    if args.download or not data_path.exists():
        download_locomo(data_path)

    if not data_path.exists():
        print(f"Error: {data_path} not found. Run with --download to fetch.", file=sys.stderr)
        sys.exit(1)

    # Remove existing eval DB for clean run
    if db_path.exists():
        db_path.unlink()
        print(f"Removed existing {db_path}")
    for suffix in [".hnsw"]:
        p = db_path.with_suffix(suffix)
        if p.exists():
            p.unlink()
    for suffix in ["-wal", "-shm"]:
        p = Path(str(db_path) + suffix)
        if p.exists():
            p.unlink()

    db_path.parent.mkdir(parents=True, exist_ok=True)
    conn = init_db(db_path)

    with open(data_path) as f:
        data = json.load(f)

    total_windows = 0
    for entry in data:
        sample_id = entry.get("sample_id", "?")
        conversation = entry.get("conversation", {})
        sessions = extract_sessions(conversation)

        speaker_a = conversation.get("speaker_a", "A")
        speaker_b = conversation.get("speaker_b", "B")
        n_sessions = len(sessions)
        n_turns = sum(len(s["turns"]) for s in sessions)

        print(f"{sample_id}: {speaker_a} & {speaker_b} — {n_sessions} sessions, {n_turns} turns", end="")

        windows = window_turns(sessions, args.window)
        for i, window in enumerate(windows):
            title = f"{sample_id} window {i}"
            insert_memory(conn, title, window)
            total_windows += 1

        conn.commit()
        print(f" → {len(windows)} windows")

    conn.close()
    print(f"\nTotal: {total_windows} windows ingested into {db_path}")

    # Export QA mapping for benchmark
    qa_map = []
    for entry in data:
        sample_id = entry.get("sample_id", "?")
        for qa in entry.get("qa", []):
            qa_map.append({
                "sample_id": sample_id,
                "question": qa["question"],
                "answer": qa.get("answer", "Not mentioned in the conversation"),
                "evidence": qa.get("evidence", []),
                "category": qa.get("category", 0),
                "adversarial_answer": qa.get("adversarial_answer", ""),
            })

    qa_path = data_path.parent / "qa_map.json"
    with open(qa_path, "w") as f:
        json.dump(qa_map, f, indent=2)
    print(f"QA map: {len(qa_map)} questions → {qa_path}")


if __name__ == "__main__":
    main()
