FROM alpine:3.17 AS certs
FROM scratch
ENTRYPOINT ["/usr/local/bin/minecraft-preempt"]

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY minecraft-preempt /usr/local/bin/
