FROM gcr.io/k8s-prow/git:0.2
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
	go \
	libc-dev \
	lsof \
	procps \
	strace \
	docker \
	jq \
	musl-dev \
	linux-headers

RUN curl -sSL https://sdk.cloud.google.com | bash

ADD dockerize /dockerize

ENTRYPOINT ["/dockerize"]
