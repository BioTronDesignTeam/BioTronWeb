# Ubuntu 24.04 development image: Node 22, git, and native-build tooling
# for Vite / React / Three.js work. Not used in production.
FROM ubuntu:24.04

ARG NODE_MAJOR=22
ARG USERNAME=vscode
ARG USER_UID=1000
ARG USER_GID=1000
ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        git \
        gnupg \
        build-essential \
        python3 \
        sudo \
        locales \
        openssh-client \
        wget \
        unzip \
        less \
        vim-tiny \
        xz-utils \
    && locale-gen en_US.UTF-8 \
    && rm -rf /var/lib/apt/lists/*

ENV LANG=en_US.UTF-8 \
    LC_ALL=en_US.UTF-8

# Node.js 22.x LTS from NodeSource (matches @types/node ^22 in package.json).
RUN curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

# ubuntu:24.04 ships a default `ubuntu` user on UID 1000; replace it with vscode.
RUN if getent passwd ubuntu >/dev/null; then userdel -r ubuntu; fi \
    && if getent group ubuntu >/dev/null; then groupdel ubuntu; fi \
    && groupadd --gid "${USER_GID}" "${USERNAME}" \
    && useradd --uid "${USER_UID}" --gid "${USER_GID}" -m "${USERNAME}" \
    && echo "${USERNAME} ALL=(root) NOPASSWD:ALL" > "/etc/sudoers.d/${USERNAME}" \
    && chmod 0440 "/etc/sudoers.d/${USERNAME}"

USER ${USERNAME}
WORKDIR /workspace
