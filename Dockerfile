# syntax=docker/dockerfile:1

FROM alpine:3.19.0
LABEL maintainer="capricornusx@gmail.com"
LABEL description="API for Stable Bloom Filter"

COPY bloom-du /bin/bloom-du

EXPOSE 8515
WORKDIR /bin/
ENTRYPOINT ["/bin/bloom-du"]


