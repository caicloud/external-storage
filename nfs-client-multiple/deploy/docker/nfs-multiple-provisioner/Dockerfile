FROM cargo.caicloudprivatetest.com/caicloud/debian:jessie

# Set the default timezone to Shanghai
RUN echo "Asia/Shanghai" > /etc/timezone
RUN dpkg-reconfigure -f noninteractive tzdata

# TODO do not rely on net
RUN apt-get update && apt-get install -y --no-install-recommends \
        netbase \
        nfs-common \
        iputils-ping \
        iproute \
        netcat \
        strace \
        vim-tiny \
	&& rm -rf /var/lib/apt/lists/*

WORKDIR /caicloud/external-storage
COPY ./bin/nfs-multiple-provisioner /caicloud/external-storage/

CMD ["/caicloud/external-storage/nfs-multiple-provisioner"]
