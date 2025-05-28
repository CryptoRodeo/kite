# The purpose of this container is to initialize the database.
FROM registry.redhat.io/ubi9/ubi-minimal@sha256:92b1d5747a93608b6adb64dfd54515c3c5a360802db4706765ff3d8470df6290

USER root

# Install only runtime dependencies
RUN microdnf install -y postgresql && \
    curl -sSf https://atlasgo.sh | sh && \
    microdnf clean all & \
    rm -rf /var/cache/yum

# Non-root user
USER 1001

WORKDIR /opt/app-root/src

# Atlas config and migrations
COPY atlas.hcl .
COPY migrations/ ./migrations/
# Entrypoint script
COPY scripts/init.sh .

ENTRYPOINT [ "init.sh" ]