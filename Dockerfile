FROM rust:1-bookworm-slim AS build
WORKDIR /src
COPY . .
RUN cargo build --release --locked

FROM debian:bookworm-slim
RUN groupadd -r grasshopper && useradd -r -g grasshopper grasshopper
COPY --from=build /src/target/release/grasshopper /usr/local/bin/
VOLUME /data
ENV GRASSHOPPER_DB=/data/brain.db
EXPOSE 8106
USER grasshopper
ENTRYPOINT ["grasshopper"]
CMD ["serve", "--port", "8106"]
