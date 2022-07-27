FROM mcr.microsoft.com/vscode/devcontainers/base:debian

# Install additional OS packages.
ARG DEBIAN_FRONTEND=noninteractive
RUN apt update && apt -y install --no-install-recommends \
    build-essential \
    python3-pip \
    golang \
    npm

# Install Python packages.
RUN pip3 install \
    pre-commit \
    detect-secrets

# Install global node packages.
RUN npm install -g markdownlint-cli
