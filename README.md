# Akita

![GitHub Discussions](https://img.shields.io/github/discussions/sarchlab/akita)


[![Go Reference](https://pkg.go.dev/badge/github.com/sarchlab/akita/v3.svg)](https://pkg.go.dev/github.com/sarchlab/akita/v3)
[![Go Report Card](https://goreportcard.com/badge/github.com/sarchlab/akita/v3)](https://goreportcard.com/report/github.com/sarchlab/akita/v3)
[![Akita Test](https://github.com/sarchlab/akita/actions/workflows/akita_test.yml/badge.svg)](https://github.com/sarchlab/akita/actions/workflows/akita_test.yml)

Akita is a computer architecture simulation engine. Like a game engine, a simulator engine is not a simulator, but rather a framework for building simulators. Akita is designed to be modular and extensible, allowing for easy experimentation with new computer architecture design ideas.



## Sub-Projects

### Akita

The simulator engine itself is located under the packages including:

* `github.com/sarchlab/akita/sim`
* `github.com/sarchlab/akita/pipelining`
* `gitlab.com/sarchlab/akita/analysis`

### AkitaRTM

AkitaRTM stands for Real-Time Monitoring (RTM) tool for Akita. It is a web-based tool that can be used to monitor the execution of a simulator developed with Akita. It is located under the `github.com/sarchlab/akita/monitoring` package.

### Daisen

Daisen is the visualization tool for Akita. It is located under the `github.com/sarchlab/akita/daisen` package. For a brief introduction to Daisen, please refer to the [github.com/sarchlab/akita/daisen](daisen) directory.

### First-Party Components

Akita provides several generic first-party components that can be used to build simulators, located under the `github.com/sarchlab/akita/mem` and `github.com/sarchlab/akita/noc` packages. As the name suggests, the `mem` package contains memory components (e.g., caches, TLB, DRAM controller), while the `noc` package contains network-on-chip components (e.g., switches).
