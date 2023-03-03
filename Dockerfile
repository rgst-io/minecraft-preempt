FROM alpine:3.17
ENTRYPOINT ["/usr/local/bin/minecraft-preempt"]
COPY minecraft-preempt /usr/local/bin/
