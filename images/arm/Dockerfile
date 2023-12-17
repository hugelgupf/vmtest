# Copyright 2018-2023 the u-root Authors. All rights reserved
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

FROM ubuntu:rolling AS base

# Install dependencies
RUN apt-get update &&                          \
    apt-get install -y --no-install-recommends \
        ca-certificates                        \
        `# Linux dependencies`                 \
        bc                                     \
        bison                                  \
        flex                                   \
        gcc                                    \
        git                                    \
        make                                   \
        gcc-arm-linux-gnueabi                  \
        `# Linux kernel build deps`            \
        libelf-dev;

WORKDIR /root

# Build linux
RUN git clone --depth=1 --branch=v6.1.68 https://git.kernel.org/pub/scm/linux/kernel/git/stable/linux.git

# Copy config file
COPY config_linux.txt linux/.config
RUN chmod 0444 linux/.config

RUN cd linux;                                         \
    export ARCH=arm;                                  \
    export CROSS_COMPILE=arm-linux-gnueabi-;          \
    make olddefconfig;                                \
    make -j$(($(nproc) * 2 + 1));

# TODO: use tagged version.
FROM ghcr.io/hugelgupf/vmtest/qemu:main AS qemu

FROM cimg/go:1.20
WORKDIR /home/circleci
COPY --from=qemu /zqemu /zqemu
COPY --from=base /root/linux/arch/arm/boot/zImage /home/circleci/zImage
ENV VMTEST_KERNEL "/home/circleci/zImage"
ENV VMTEST_ARCH "arm"
ENV VMTEST_QEMU "/zqemu/bin/qemu-system-arm -M virt,highmem=off -L /zqemu/pc-bios"
