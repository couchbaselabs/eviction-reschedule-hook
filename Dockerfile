FROM docker:24-dind

# Install Go and kubectl
RUN apk update && \
    apk add --no-cache \
      go \
      curl \
      bash && \
    # Download kubectl
    curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" && \
    install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl && \
    rm kubectl && \
    rm -rf /var/cache/apk/*
