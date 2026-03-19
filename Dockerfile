FROM gcr.io/distroless/static-debian13:nonroot
ARG TARGETPLATFORM
ENTRYPOINT ["/usr/local/bin/minecraft-preempt"]
COPY $TARGETPLATFORM/minecraft-preempt /usr/local/bin/
COPY $TARGETPLATFORM/minecraft-preempt-agent /usr/local/bin/
