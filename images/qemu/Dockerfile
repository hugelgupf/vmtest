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
        bzip2;

WORKDIR /root

# Build SLIRP
RUN git clone --depth=1 --branch=v4.7.0 https://gitlab.freedesktop.org/slirp/libslirp.git
RUN cd libslirp && meson setup build && ninja -C build install

# Build QEMU
RUN git clone --depth=1 --branch=v8.1.0-rc2 https://github.com/qemu/qemu;

RUN cd qemu && mkdir build && cd build && ../configure                 \
        --target-list=x86_64-softmmu                                   \
        --enable-virtfs                                                \
        --disable-docs                                                 \
        --disable-sdl                                                  \
        && \
      make -j$(($(nproc) * 2 + 1)) qemu-system-x86_64;

# Zapp me!
RUN git clone --depth=1 --branch=zappit https://github.com/hugelgupf/ldshim;
RUN cd ldshim && make;
RUN cd ldshim/zappit && go build;
RUN ./ldshim/zappit/zappit --shim=./ldshim/ldshim --out=./zqemu ./qemu/build/qemu-system-x86_64;

RUN cd /root;                                                           \
    cp -rL qemu/build/pc-bios/ /root/zqemu/pc-bios;

FROM scratch

COPY --from=build /root/zqemu /zqemu
ENV VMTEST_QEMU "/zqemu/bin/qemu-system-x86_64 -L /zqemu/pc-bios -m 1G"
