FROM alpine:3.21

RUN apk add --no-cache \
  bash curl socat git jq bash-completion docker-cli openssl lsof && \
  mkdir -p /etc/bash_completion.d

# inject the target os and architecture (https://docs.docker.com/reference/dockerfile/#automatic-platform-args-in-the-global-scope)
ARG TARGETARCH
ARG TARGETOS

# install kubectl
RUN curl -fsSL "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/$TARGETARCH/kubectl" > /tmp/kubectl && \
  install -o root -g root -m 0755 /tmp/kubectl /usr/local/bin/kubectl && \
  kubectl completion bash > /etc/bash_completion.d/kubectl && \
  rm /tmp/kubectl

# install helm - latest stable version (https://github.com/helm/helm/releases)
RUN curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash && \
  helm completion bash > /etc/bash_completion.d/helm

# install kind https://kind.sigs.k8s.io/docs/user/quick-start/#installing-from-release-binaries
RUN curl -fsSL https://kind.sigs.k8s.io/dl/v0.27.0/kind-linux-${TARGETARCH} > /tmp/kind && \
  install -o root -g root -m 0755 /tmp/kind /usr/local/bin/kind && \
  rm /tmp/kind

ENV KUBECONFIG="/state/kube/config-internal.yaml"

COPY install-helpers.sh /app/install-helpers.sh
RUN chmod +x /app/install-helpers.sh

COPY kind-config.yaml /app/kind-config.yaml

COPY install.sh /app/install.sh
RUN chmod +x /app/install.sh

COPY uninstall.sh /app/uninstall.sh
RUN chmod +x /app/uninstall.sh

COPY port-forward.sh /app/port-forward.sh
RUN chmod +x /app/port-forward.sh

COPY stop-port-forward.sh /app/stop-port-forward.sh
RUN chmod +x /app/stop-port-forward.sh

COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh
ENV PATH="/usr/local/bin:${PATH}"

# Set custom shell prompt with meaningful hostname
RUN echo 'export PS1="amp-quick-start:\w# "' >> /etc/profile

WORKDIR /app

ENTRYPOINT ["/app/entrypoint.sh"]