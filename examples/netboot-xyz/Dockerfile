FROM ubuntu:noble

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && \
    apt-get install -y nginx wget && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /var/www/html

RUN wget -q https://boot.netboot.xyz/ipxe/netboot.xyz.kpxe && \
    wget -q https://boot.netboot.xyz/ipxe/netboot.xyz-undionly.kpxe && \
    wget -q https://boot.netboot.xyz/ipxe/netboot.xyz.efi && \
    wget -q https://boot.netboot.xyz/ipxe/netboot.xyz.iso

COPY index.html /var/www/html/

EXPOSE 80
