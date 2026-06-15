# Hermetic, pinned build of both reference DRAM simulators.
#
# Build:  docker build -t dram-oracles mem/dram/validation/oracles
# Use:    the run script (../run_oracles.sh --docker) mounts the validation
#         dir and invokes the oracle binaries inside this image, so the exact
#         compiler/toolchain that produced ../data/reference.csv is reproduced
#         on any host with Docker — no host C++ toolchain required.
FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
        git cmake g++ make ca-certificates python3 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /opt/oracles
COPY COMMITS.txt build_oracles.sh ./
RUN chmod +x build_oracles.sh && ./build_oracles.sh /opt/oracles/build

# Convenience symlinks to the two trace frontends.
RUN ln -s /opt/oracles/build/DRAMsim3/build/dramsim3main /usr/local/bin/dramsim3main \
 && ln -s /opt/oracles/build/ramulator2/build/ramulator2 /usr/local/bin/ramulator2

WORKDIR /work
