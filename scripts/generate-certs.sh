#!/bin/bash

# Create certs directory if it doesn't exist
mkdir -p certs

# Generate CA private key using RSA
openssl genrsa -out certs/ca.key 2048

# Generate CA certificate
openssl req -new -x509 -days 365 -key certs/ca.key \
    -subj "/CN=reschedule-hook-ca" \
    -out certs/ca.crt

# Generate server private key using RSA
openssl genrsa -out certs/server.key 2048

# Create a config file for the CSR (including SANs)
cat > certs/csr.conf << EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
prompt = no

[req_distinguished_name]
CN = reschedule-hook-server.default.svc

[v3_req]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = reschedule-hook-server
DNS.2 = reschedule-hook-server.default
DNS.3 = reschedule-hook-server.default.svc
DNS.4 = reschedule-hook-server.default.svc.cluster.local
EOF

# Generate server CSR using the config
openssl req -new -key certs/server.key \
    -out certs/server.csr \
    -config certs/csr.conf

# Create a separate config file for signing (with SANs)
cat > certs/signing.conf << EOF
[ v3_ext ]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[ alt_names ]
DNS.1 = reschedule-hook-server
DNS.2 = reschedule-hook-server.default
DNS.3 = reschedule-hook-server.default.svc
DNS.4 = reschedule-hook-server.default.svc.cluster.local
EOF

# Sign the server certificate with the CA and include SANs
openssl x509 -req -in certs/server.csr \
    -CA certs/ca.crt \
    -CAkey certs/ca.key \
    -CAcreateserial \
    -out certs/server.crt \
    -days 365 \
    -extensions v3_ext \
    -extfile certs/signing.conf

# Verify the certificate to check SANs are present
openssl x509 -in certs/server.crt -text -noout

# Delete any existing secret
kubectl delete secret reschedule-hook-tls

# Create a Kubernetes secret with the certificates
kubectl create secret tls reschedule-hook-tls \
    --cert=certs/server.crt \
    --key=certs/server.key

# Get the CA certificate in base64 format for the webhook configuration
CA_BUNDLE=$(cat certs/ca.crt | base64 | tr -d '\n')

echo "Loading CA bundle into webhook configuration"
# Update the webhook configuration with the CA bundle
sed -i.bak "s/caBundle: \"\"/caBundle: \"${CA_BUNDLE}\"/" deploy/validating-webhook-config.yaml
rm -f deploy/validating-webhook-config.yaml.bak

echo "Updated webhook configuration with new CA bundle"

# Clean up the certs directory
rm -rf certs