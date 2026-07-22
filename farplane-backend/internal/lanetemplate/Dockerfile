# Farplane default Lane computer image.
# Credential-free: API keys are injected at create/start via Runtime.InjectSecrets.

FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive \
    LANG=C.UTF-8

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        git \
        build-essential \
        jq \
        unzip \
        gnupg \
        python3 \
        python3-pip \
        python3-venv \
        ruby \
        ruby-dev \
        rubygems \
    && rm -rf /var/lib/apt/lists/*

# Node.js 22 from NodeSource (avoids compiling Ruby/OpenSSL via mise in Docker).
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/* \
    && gem install bundler --no-document

# Agent CLIs (fork the template to pin versions harder).
RUN npm install -g @anthropic-ai/claude-code @openai/codex opencode-ai \
    && npm install -g @oh-my-pi/pi-coding-agent || npm install -g oh-my-pi || true

WORKDIR /workspace

COPY bridge /opt/farplane/bridge
RUN chmod +x /opt/farplane/bridge/bridge.js \
    && cd /opt/farplane/bridge && npm install --omit=dev

EXPOSE 7420

ENTRYPOINT ["node", "/opt/farplane/bridge/bridge.js"]
