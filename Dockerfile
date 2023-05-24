FROM alpine:3.18
ENTRYPOINT ["/usr/local/bin/minecraft-preempt"]
COPY minecraft-preempt /usr/local/bin/
