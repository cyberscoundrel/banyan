# Banyan Node - Remote Testing (Portable)
#
# Expects a pre-built Linux banyan binary and remote-testing.json config in
# the build context.
#
# --- Flash drive / flat directory usage ---
#   Place banyan (binary), remote-testing.json, and this Dockerfile together:
#     docker build -t banyan-remote .
#     docker run -p 9000:9000 banyan-remote
#
# --- Full deploy/ folder structure usage ---
#   From the deploy/ directory (config lives in configs/):
#     cp configs/remote-testing.json docker/remote-testing/
#     docker build -t banyan-remote docker/remote-testing/
#     docker run -p 9000:9000 banyan-remote
#

FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY banyan ./banyan
RUN chmod +x ./banyan

RUN mkdir -p /app/config
COPY remote-testing.json /app/config/remote-testing.json

EXPOSE 9000/tcp
EXPOSE 9000/udp

ENTRYPOINT ["./banyan", "-config=/app/config/remote-testing.json"]
