FROM debian:jessie

RUN apt-get update && apt-get install --yes ca-certificates

COPY /.build/artifacts/eip-controller /usr/bin/eip-controller

CMD /usr/bin/eip-controller

