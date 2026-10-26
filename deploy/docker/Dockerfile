FROM debian:trixie-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl ffmpeg \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 layerhub \
    && useradd --uid 10001 --gid 10001 --home-dir /var/lib/layerhub --create-home --shell /usr/sbin/nologin layerhub \
    && install -d -m 700 -o layerhub -g layerhub /var/lib/layerhub \
    && install -d -m 755 /opt/layerhub
COPY --chmod=755 binaries/linux-amd64/layerhub /usr/local/bin/layerhub
COPY source/web /opt/layerhub/web
COPY LICENSE /opt/layerhub/LICENSE
COPY THIRD_PARTY_NOTICES.md /opt/layerhub/THIRD_PARTY_NOTICES.md
USER 10001:10001
WORKDIR /opt/layerhub
ENV LAYERHUB_ADDRESS=0.0.0.0:5181 LAYERHUB_DATA_DIR=/var/lib/layerhub/data LAYERHUB_WEB_DIR=/opt/layerhub/web LAYERHUB_DEVICE_MODE=offline LAYERHUB_ENABLE_PRINT_START=0
EXPOSE 5181
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 CMD curl --fail --silent http://127.0.0.1:5181/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/layerhub"]
