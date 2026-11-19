# Grasshopper

Grasshopper is one person's memory for every AI agent they use, on every machine. It keeps short, curated statements worth carrying between sessions and points to where everything else lives.

## Language

**Owner**:
The one person whose memory Grasshopper holds and who decides what is kept.
_Avoid_: user, admin, account

**Memory**:
A short statement worth carrying between sessions: a fact, preference, decision with its reason, lesson, or handoff.
_Avoid_: note, record, chunk, entry

**Kept memory**:
A memory the owner has approved. Only kept memories load into a session on their own.
_Avoid_: confirmed memory, approved memory

**Suggestion**:
A memory an agent saved that the owner has not kept yet. It can be found by search but never loads on its own.
_Avoid_: unconfirmed memory, draft, unreviewed memory

**Document**:
Content that lives in its own system, such as notes, files, transcripts, email or a repository. Grasshopper never stores documents; agents reach them through their own tools.
_Avoid_: knowledge, corpus, file

**Pointer**:
A memory that says where a document lives and what it is for.
_Avoid_: link, reference, bookmark

**Handoff**:
A short memory that says where a project's work stands by pointing to its task file and last commit. It stops loading after a short time.
_Avoid_: summary, status, progress note

**Preference**:
A memory about what the owner likes or chooses, such as a favorite fragrance or a preferred tool.
_Avoid_: rule, setting

**Rule**:
An instruction for how agents must behave. Rules live in instruction files that agents load on their own, never in memory.
_Avoid_: preference, policy, guideline

## Where memories belong

**Facet**:
One independent way a memory is classified. A memory has an area and may also have a project and a device.
_Avoid_: scope, tag, label

**Area**:
A broad part of the owner's life or work, such as home, money or work. Every memory belongs to the one area it serves, not the activity that produced it.
_Avoid_: category, domain, space, context

**Project**:
One piece of work with its own folder, usually a repository. A project draws on the areas it names.
_Avoid_: repo, workspace

**Everywhere**:
The small set of memories that load into every session, whatever its project or area.
_Avoid_: global, default

**Session**:
One conversation with an agent from start to finish, such as a thread in T3 Code. A session with no project loads only the everywhere set.
_Avoid_: chat, thread, run
