FROM golang:1.10.4-alpine
LABEL maintainer="daegeun.kim@boscoin.io"

RUN apk add --no-cache \
	bash \
	binutils \
	build-base \
	ca-certificates \
	curl \
	gcc \
	gdb \
	gfortran \
	git \
	libc-dev \
	lsof \
	openssh \
	procps \
	strace \
	docker \
	jq \
	musl-dev \
	linux-headers \
	&& update-ca-certificates

RUN curl -sSL https://sdk.cloud.google.com | bash

ADD dockerize /dockerize

ENTRYPOINT ["/dockerize"]
