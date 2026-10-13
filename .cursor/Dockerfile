FROM ubuntu:26.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        git \
        sudo \
        build-essential \
        gettext \
        libgtk-4-1 \
        libadwaita-1-0 \
        xvfb \
        ubuntu-dbgsym-keyring \
    && printf 'ubuntu ALL=(ALL) NOPASSWD:ALL\n' > /etc/sudoers.d/ubuntu \
    && chmod 440 /etc/sudoers.d/ubuntu \
    && . /etc/os-release \
    && printf 'deb http://ddebs.ubuntu.com %s main universe\ndeb http://ddebs.ubuntu.com %s-updates main universe\n' \
        "$VERSION_CODENAME" "$VERSION_CODENAME" > /etc/apt/sources.list.d/ddebs.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
        libpango-1.0-0-dbgsym \
        libpangoft2-1.0-0-dbgsym \
        libpangocairo-1.0-0-dbgsym \
        libfontconfig1-dbgsym \
    && rm -rf /var/lib/apt/lists/*

ARG GO_VERSION=1.27.2
ARG GO_SHA256=ecbadb99091a3f46e31f5f934b068b1864eafa7995211b39eaddf76996045fe5
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tgz \
    && echo "${GO_SHA256}  /tmp/go.tgz" | sha256sum -c - \
    && tar -C /usr/local -xzf /tmp/go.tgz \
    && rm /tmp/go.tgz \
    && ln -sf /usr/local/go/bin/go /usr/local/bin/go \
    && ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt

USER ubuntu
