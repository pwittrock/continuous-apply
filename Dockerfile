# Build and test the manager binary
FROM golang:1.10.3 as builder
WORKDIR /go/src/github.com/pwittrock/continuous-apply
COPY pkg/    pkg/
COPY cmd/    cmd/
COPY vendor/ vendor/
RUN CGO_ENABLED=0 GOOS=$OS GOARCH=$ARCH go build -o manager github.com/pwittrock/continuous-apply/cmd/manager
RUN CGO_ENABLED=0 GOOS=$OS GOARCH=$ARCH go build -o continuous-apply github.com/pwittrock/continuous-apply/cmd/continuous-apply

FROM golang:1.10.3
RUN apt-get update && apt-get install -y apt-transport-https
RUN apt-get install git curl -y
RUN curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -
RUN touch /etc/apt/sources.list.d/kubernetes.list
RUN echo "deb http://apt.kubernetes.io/ kubernetes-xenial main" | tee -a /etc/apt/sources.list.d/kubernetes.list
RUN apt-get update
RUN apt-get install kubectl -y
ENV VERSION 1.10.3
ENV OS linux
ENV ARCH amd64
RUN curl https://dl.google.com/go/go$VERSION.$OS-$ARCH.tar.gz --output go$VERSION.$OS-$ARCH.tar.gz
RUN tar -C /usr/local -xzf go$VERSION.$OS-$ARCH.tar.gz
ENV PATH $PATH:/usr/local/go/bin
RUN go get github.com/kubernetes-sigs/kustomize

WORKDIR /root/
COPY --from=builder /go/src/github.com/pwittrock/continuous-apply/manager /usr/local/bin/manager
COPY --from=builder /go/src/github.com/pwittrock/continuous-apply/continuous-apply ./continuous-apply
