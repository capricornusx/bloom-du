# syntax=docker/dockerfile:1

FROM alpine:3.19.0
LABEL maintainer="capricornusx@gmail.com"
LABEL description="API for Stable Bloom Filter"

COPY bloom-du /opt/bloom-du/bloom-du

EXPOSE 8515
WORKDIR /opt/
ENTRYPOINT ["/opt/bloom-du"]


