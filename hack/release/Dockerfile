FROM k8s.gcr.io/debian-base:v1.0.0

RUN apt-get update -y && apt-get install -y bash jq curl

COPY common.sh /common.sh
COPY verify-emoji.sh /verify-emoji.sh

ENTRYPOINT ["/verify-emoji.sh"]
