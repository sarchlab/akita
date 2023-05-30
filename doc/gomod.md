# Use Go Modules

In the past, developing Go programs relies on GOPATH to organize dependencies. However, GOPATH can cause different results for different execution environment. Also, GO Module will be enablesd by default in Go 1.13. Therefore, we favor Go Modules instead of GOPATH.

## User an Akita Series Simualtor

Let's explain the workflow assuming if you want to use the "gcn3" simulator. You would need to download the repository to any place on your hard driver other than your GOPATH. In the "gcn3" repo, you do not need to run `go get`. Instead, simply go to any directory that contains a main program and run `go build`. This command will download the dependencies automatically. Since the dependency versions are explicitely written in the `go.mod` file, every user should download the same version of dependeicies and hence, produce the same result when running the simulation.

## Start a new repo for experiments

Experiments usually require the user to start a new repo to hold the starting code. When creating such a repo, the user should create a `go.mod` file at the root of the experiment repo, containing information similar to the following example:

```gomod
module gitlab.com/syifan/multi_gpu_experiments

require (
	github.com/sarchlab/akita v1.2.3
	gitlab.com/akita/gcn3 v1.2.2
	gitlab.com/akita/mem v1.1.2
	gitlab.com/akita/noc v1.1.1
)
```

As all other users who wants to rerun the experiments will download the same `go.mod` file, they can guarantee to use the same library versions and compile the same executable. And they can guarantee generating the same results. 

## Working with multiple repos

It is common that you may need to make some changes across mutliple repos and Go Modules can handle this.

First, I do recommend to think again if editing multiple repos is necessary. Since there is no cyclic dependency, you may always update one repo, commit it and merge it first and then work on the repo that requires this change. 

However, editing multiple repos sequentially is not always a solution. For example, you may just need to add a print in the `mem` repo to check if the memory accesses actually happen. In this case, you can use the `replace` directive to specify a local version of a library. For example, you can change the example above to this:

```gomod
module gitlab.com/syifan/multi_gpu_experiments

require (
	github.com/sarchlab/akita v1.2.3
	gitlab.com/akita/gcn3 v1.2.2
	gitlab.com/akita/mem v1.1.2
	gitlab.com/akita/noc v1.1.1
)

replace gitlab.com/akita/mem /home/username/path/to/your/mem/repo
```

When you commit, simply do not commit the `go.mod` file. Or you can remove this line before you commit.
