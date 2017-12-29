FROM cargo.caicloudprivatetest.com/caicloud/debian:jessie

# Set the default timezone to Shanghai
RUN echo "Asia/Shanghai" > /etc/timezone
RUN dpkg-reconfigure -f noninteractive tzdata

WORKDIR /caicloud/external-storage
COPY ./bin/nfs-multiple-provisioner /caicloud/external-storage/

CMD ["/caicloud/external-storage/nfs-multiple-provisioner"]
