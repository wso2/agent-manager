FROM public.ecr.aws/docker/library/alpine:3.24

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

# Build context is the repo root (see release-config.json: amp-quick-start
# sets "context": "."), so paths below are repo-relative. This lets the image
# bundle the deployment resources the installer applies (values/, manifests,
# helper scripts) instead of fetching them from raw.githubusercontent.com at
# install time — GitHub rate-limits unauthenticated per-IP requests with HTTP
# 429, and helm/kubectl do not retry, which aborts the install.
COPY --chown=wso2-amp:wso2-amp deployments/quick-start/.bash_profile ${USER_HOME}/.bash_profile
COPY --chown=wso2-amp:wso2-amp deployments/quick-start/.bashrc ${USER_HOME}/.bashrc
# Copy scripts and configs
COPY --chown=wso2-amp:wso2-amp deployments/quick-start/install.sh ${USER_HOME}/install.sh
COPY --chown=wso2-amp:wso2-amp deployments/quick-start/install-helpers.sh ${USER_HOME}/install-helpers.sh
COPY --chown=wso2-amp:wso2-amp deployments/quick-start/uninstall.sh ${USER_HOME}/uninstall.sh
COPY --chown=wso2-amp:wso2-amp deployments/quick-start/k3d-config.yaml ${USER_HOME}/k3d-config.yaml
COPY --chown=root:root deployments/quick-start/entrypoint.sh /entrypoint.sh

# Bundle deployment resources next to the scripts (flat layout). install.sh
# resolves these under ${SCRIPT_DIR} when the parent-relative repo layout is
# absent (see DEPLOYMENTS_DIR in install.sh).
COPY --chown=wso2-amp:wso2-amp deployments/single-cluster/ ${USER_HOME}/single-cluster/
COPY --chown=wso2-amp:wso2-amp deployments/values/ ${USER_HOME}/values/
COPY --chown=wso2-amp:wso2-amp deployments/k8s/coredns-amp-custom.yaml ${USER_HOME}/k8s/coredns-amp-custom.yaml
# Whole scripts/ dir: add-environment-thunder.sh sources its sibling
# thunder-naming.sh, so both (and the other env helpers) must ship together.
COPY --chown=wso2-amp:wso2-amp deployments/scripts/ ${USER_HOME}/scripts/
RUN chmod +x ${USER_HOME}/install.sh && \
    chmod +x /entrypoint.sh && \
    chmod +x ${USER_HOME}/install-helpers.sh && \
    chmod +x ${USER_HOME}/uninstall.sh

# Set working directory
WORKDIR ${USER_HOME}

# Set entrypoint
ENTRYPOINT ["/entrypoint.sh"]

