# Copyright 2018-2023 the u-root Authors. All rights reserved
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

FROM ubuntu:rolling AS base

# Install dependencies
RUN apt-get update &&                          \
    apt-get install -y --no-install-recommends \
        ca-certificates                             \
        `# Linux dependencies`                      \
        bc                                          \
        bison                                       \
        flex                                        \
        gcc                                         \
        git                                         \
        make                                        \
        `# Linux kernel build deps`                 \
        libelf-dev;

# Create working directory
WORKDIR /root

# Build linux
RUN git clone --depth=1 --branch=v6.0 https://github.com/torvalds/linux;

COPY config_linux.txt linux/.config
RUN chmod 0444 linux/.config;

RUN cd linux;                     \
    make olddefconfig;            \
    make -j$(($(nproc) * 2 + 1));

# TODO: use tagged version.
FROM ghcr.io/hugelgupf/vmtest/qemu:main AS qemu

FROM cimg/go:1.20
WORKDIR /home/circleci
COPY --from=qemu /zqemu /zqemu
COPY --from=base /root/linux/arch/x86_64/boot/bzImage /home/circleci/bzImage
ENV VMTEST_KERNEL "/home/circleci/bzImage"
ENV VMTEST_QEMU "/zqemu/bin/qemu-system-x86_64 -L /zqemu/pc-bios -m 1G"
