# Copyright 2018-2023 the u-root Authors. All rights reserved
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

FROM ubuntu:rolling AS build

# Install dependencies
RUN apt-get update &&                          \
    apt-get install -y --no-install-recommends \
        gcc                                         \
        git                                         \
        make                                        \
        `# QEMU dependencies`                       \
        libattr1-dev                                \
        libcap-dev                                  \
        libcap-ng-dev                               \
        libfdt-dev                                  \
        libglib2.0-dev                              \
        libpixman-1-dev                             \
        libssh-dev                                  \
        libpmem-dev                                 \
        libusb-dev                                  \
        libcurl4-openssl-dev                        \
        libtasn1-6-dev                              \
        libiscsi-dev                                \
        libglusterfs-dev                            \
        libnfs-dev                                  \
        libbpf-dev                                  \
        meson                                       \
        ninja-build                                 \
        python3                                     \
        python3-pip                                 \
        zlib1g-dev                                  \
        golang                                      \
        bzip2                                       \
        # Compiling Option ROMs
        gcc-aarch64-linux-gnu                       \
        gcc-arm-linux-gnueabi                       \
        gcc-i686-linux-gnu                          \
        gcc-riscv64-linux-gnu;

WORKDIR /root

# Build SLIRP
RUN git clone --depth=1 --branch=v4.7.0 https://gitlab.freedesktop.org/slirp/libslirp.git
RUN cd libslirp && meson setup build && ninja -C build install

# Build QEMU
RUN git clone --depth=1 --recurse-submodules --branch=v8.2.0 https://github.com/qemu/qemu;

RUN cd qemu && mkdir build && cd build && ../configure                 \
        --target-list="x86_64-softmmu aarch64-softmmu arm-softmmu riscv64-softmmu ppc64-softmmu" \
        --enable-virtfs                                                \
        --enable-slirp                                                 \
        --disable-docs                                                 \
        --disable-sdl                                                  \
        && \
      make -j$(($(nproc) * 2 + 1)) \
        pc-bios/optionrom/all     \
        pc-bios/edk2-aarch64-code.fd \
        pc-bios/edk2-arm-code.fd \
        pc-bios/edk2-arm-vars.fd \
        pc-bios/edk2-i386-code.fd \
        pc-bios/edk2-i386-secure-code.fd \
        pc-bios/edk2-i386-vars.fd \
        pc-bios/edk2-x86_64-code.fd \
        pc-bios/edk2-x86_64-secure-code.fd \
        pc-bios/bamboo.dtb \
        pc-bios/canyonlands.dtb \
        pc-bios/petalogix-s3adsp1800.dtb \
        pc-bios/petalogix-ml605.dtb \
        fsdev/virtfs-proxy-helper \
        qemu-system-x86_64        \
        qemu-system-aarch64       \
        qemu-system-arm           \
        qemu-system-riscv64       \
        qemu-system-ppc64         \
        qemu-bridge-helper        \
        qemu-img                  \
        qemu-nbd;

RUN git clone --depth=1 --branch=zappit https://github.com/hugelgupf/ldshim;

RUN cd ldshim && make;
RUN cd ldshim/zappit && go build;

# Zapp em!
RUN ./ldshim/zappit/zappit --shim=./ldshim/ldshim --out=./zqemu ./qemu/build/qemu-system-x86_64;
RUN ./ldshim/zappit/zappit --shim=./ldshim/ldshim --out=./zqemu ./qemu/build/qemu-system-aarch64;
RUN ./ldshim/zappit/zappit --shim=./ldshim/ldshim --out=./zqemu ./qemu/build/qemu-system-arm;
RUN ./ldshim/zappit/zappit --shim=./ldshim/ldshim --out=./zqemu ./qemu/build/qemu-system-riscv64;
RUN ./ldshim/zappit/zappit --shim=./ldshim/ldshim --out=./zqemu ./qemu/build/qemu-system-ppc64;
RUN ./ldshim/zappit/zappit --shim=./ldshim/ldshim --out=./zqemu ./qemu/build/qemu-img;
RUN ./ldshim/zappit/zappit --shim=./ldshim/ldshim --out=./zqemu ./qemu/build/qemu-nbd;
RUN ./ldshim/zappit/zappit --shim=./ldshim/ldshim --out=./zqemu ./qemu/build/qemu-bridge-helper;

RUN cp -rL /root/qemu/build/pc-bios/ /root/zqemu/pc-bios;
RUN cp -rL /root/qemu/pc-bios/* /root/zqemu/pc-bios;

FROM scratch

COPY --from=build /root/zqemu /zqemu
