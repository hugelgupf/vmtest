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
RUN git clone --depth=1 --recurse-submodules --branch=v8.1.0-rc2 https://github.com/qemu/qemu;

RUN cd qemu && mkdir build && cd build && ../configure                 \
        --target-list="x86_64-softmmu aarch64-softmmu arm-softmmu riscv64-softmmu" \
        --enable-virtfs                                                \
        --enable-slirp                                                 \
        --disable-docs                                                 \
        --disable-sdl                                                  \
        && \
      make -j$(($(nproc) * 2 + 1)) \
        pc-bios/optionrom/all     \
        fsdev/virtfs-proxy-helper \
        qemu-system-x86_64        \
        qemu-system-aarch64       \
        qemu-system-arm           \
        qemu-system-riscv64;

RUN git clone --depth=1 --branch=zappit https://github.com/hugelgupf/ldshim;
RUN cd ldshim && make;
RUN cd ldshim/zappit && go build;

# Zapp em!
RUN ./ldshim/zappit/zappit --shim=./ldshim/ldshim --out=./zqemu ./qemu/build/qemu-system-x86_64;
RUN ./ldshim/zappit/zappit --shim=./ldshim/ldshim --out=./zqemu ./qemu/build/qemu-system-aarch64;
RUN ./ldshim/zappit/zappit --shim=./ldshim/ldshim --out=./zqemu ./qemu/build/qemu-system-arm;
RUN ./ldshim/zappit/zappit --shim=./ldshim/ldshim --out=./zqemu ./qemu/build/qemu-system-riscv64;

RUN cp -rL /root/qemu/build/pc-bios/ /root/zqemu/pc-bios;
RUN cp -rL /root/qemu/pc-bios/* /root/zqemu/pc-bios;

FROM scratch

COPY --from=build /root/zqemu /zqemu
