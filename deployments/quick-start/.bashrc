# OpenChoreo Quick-Start Shell Configuration

# Locate kubectl, helm, and k3d in the login shell.
export PATH=/usr/local/bin:$PATH

# Custom prompt with colors
export PS1="\[\033[01;32m\]wso2-amp\[\033[00m\]:\[\033[01;34m\]\w\[\033[00m\]\$ "

# Enable bash completion
if [ -f /etc/bash_completion ]; then
    . /etc/bash_completion
fi

# Source shared configuration
CLUSTER_NAME="amp-local"
KUBECONFIG_PATH="$HOME/.kube/config"

# Helpful aliases
alias k="kubectl"
alias kgp="kubectl get pods"
alias kgs="kubectl get svc"
alias kgn="kubectl get nodes"
alias kga="kubectl get all -A"
alias ll="ls -lah"

# Setup kubeconfig if k3d cluster exists
if k3d cluster list 2>/dev/null | grep -q "^${CLUSTER_NAME} "; then
    # Only merge if kubeconfig doesn't already have the context
    if ! kubectl config get-contexts "k3d-${CLUSTER_NAME}" &>/dev/null; then
        mkdir -p ~/.kube
        k3d kubeconfig merge "${CLUSTER_NAME}" --kubeconfig-merge-default &>/dev/null
    fi
fi
