#!/usr/bin/env bash
set -eu

# Use DEBUG=1 ./scripts/download-binaries.sh to get debug output
quiet="-s"
[[ -z "${DEBUG:-""}" ]] || {
  set -x
  quiet=""
}

# Use BASE_URL=https://my/binaries/url ./scripts/download-binaries to download
# from a different bucket
: "${BASE_URL:="https://storage.googleapis.com/k8s-c10s-test-binaries"}"

test_framework_dir="$(cd "$(dirname "$0")/.." ; pwd)"
os="$(uname -s)"
os_lowercase="$(echo "$os" | tr '[:upper:]' '[:lower:]' )"
arch="$(uname -m)"

echo "About to download a couple of binaries. This might take a while..."
curl $quiet "${BASE_URL}/etcd-${os}-${arch}" --output "${test_framework_dir}/assets/bin/etcd"
curl $quiet "${BASE_URL}/kube-apiserver-${os}-${arch}" --output "${test_framework_dir}/assets/bin/kube-apiserver"

kubectl_version="$(curl https://storage.googleapis.com/kubernetes-release/release/stable.txt)"
kubectl_url="https://storage.googleapis.com/kubernetes-release/release/${kubectl_version}/bin/${os_lowercase}/amd64/kubectl"
curl ${quiet} "$kubectl_url" --output "${test_framework_dir}/assets/bin/kubectl"

chmod +x "${test_framework_dir}/assets/bin/etcd"
chmod +x "${test_framework_dir}/assets/bin/kube-apiserver"
chmod +x "${test_framework_dir}/assets/bin/kubectl"

echo "Done!"
