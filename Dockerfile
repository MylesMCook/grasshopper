FROM rust:slim-bookworm AS builder
WORKDIR /build
RUN apt-get update && apt-get install -y pkg-config && rm -rf /var/lib/apt/lists/*
COPY Cargo.toml Cargo.lock ./
COPY src/ src/
COPY tests/ tests/
RUN cargo build --release --locked

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /build/target/release/grasshopper /usr/local/bin/grasshopper
EXPOSE 8106
VOLUME ["/data", "/root/.cache/grasshopper/models"]
ENTRYPOINT ["grasshopper"]
CMD ["serve", "--port", "8106", "--db", "/data/brain.db"]
