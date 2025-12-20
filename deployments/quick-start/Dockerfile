FROM alpine:3.21

# Version arguments for reproducibility and easy updates
ARG KUBECTL_VERSION=v1.32.10
ARG HELM_VERSION=v3.19.4
ARG K3D_VERSION=v5.8.3

# Inject the target os and architecture
ARG TARGETARCH
ARG TARGETOS
ARG USER_HOME=/home/wso2-amp

# Install all base packages
RUN apk add --no-cache \
  bash curl socat git jq yq bash-completion docker-cli openssl \
  bind-tools netcat-openbsd tcpdump \
  htop procps \
  vim \
  sudo \
  wget ca-certificates ncurses && \
  mkdir -p /etc/bash_completion.d

# Install kubectl, helm, and k3d with completions
RUN curl -fsSL "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" > /tmp/kubectl && \
  install -o root -g root -m 0755 /tmp/kubectl /usr/local/bin/kubectl && \
  kubectl completion bash > /etc/bash_completion.d/kubectl && \
  rm /tmp/kubectl && \
  curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | DESIRED_VERSION=${HELM_VERSION} bash && \
  helm completion bash > /etc/bash_completion.d/helm && \
  curl -fsSL https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | TAG=${K3D_VERSION} bash && \
  k3d completion bash > /etc/bash_completion.d/k3d

# Create user
RUN adduser -D -h ${USER_HOME} -s /bin/bash wso2-amp && \
    echo "wso2-amp ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/wso2-amp && \
    chmod 0440 /etc/sudoers.d/wso2-amp

COPY --chown=wso2-amp:wso2-amp .bash_profile ${USER_HOME}/.bash_profile
COPY --chown=wso2-amp:wso2-amp .bashrc ${USER_HOME}/.bashrc
# Copy scripts and configs
COPY --chown=wso2-amp:wso2-amp install.sh ${USER_HOME}/install.sh
COPY --chown=wso2-amp:wso2-amp install-helpers.sh ${USER_HOME}/install-helpers.sh
COPY --chown=wso2-amp:wso2-amp uninstall.sh ${USER_HOME}/uninstall.sh
COPY --chown=wso2-amp:wso2-amp k3d-config.yaml ${USER_HOME}/k3d-config.yaml
COPY --chown=root:root entrypoint.sh /entrypoint.sh
RUN chmod +x ${USER_HOME}/install.sh && \
    chmod +x /entrypoint.sh && \
    chmod +x ${USER_HOME}/install-helpers.sh && \
    chmod +x ${USER_HOME}/uninstall.sh

# Set working directory
WORKDIR ${USER_HOME}

# Set entrypoint
ENTRYPOINT ["/entrypoint.sh"]

