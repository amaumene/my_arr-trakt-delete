FROM registry.access.redhat.com/ubi9/go-toolset AS builder

COPY ./src/* .

RUN go build

FROM registry.access.redhat.com/ubi9/ubi-minimal

COPY --from=builder /opt/app-root/src/my_arr-trakt-delete /app/my_arr-trakt-delete

USER 1001

VOLUME /data

CMD [ "/app/my_arr-trakt-delete" ]
