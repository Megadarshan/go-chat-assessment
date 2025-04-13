FROM alpine:latest AS builder

RUN mkdir /app

COPY goChatApp /app

CMD [ "/app/goChatApp"]

FROM scratch

COPY --from=builder /app/goChatApp /goChatApp

EXPOSE 8080

CMD ["/goChatApp"]