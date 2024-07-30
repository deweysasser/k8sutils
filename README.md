
# k8sutils

A series of command line utils that allow me to manipulate the cluster components in a more human friendly way.

# Installation

Mac: 

    `brew install deweysasser/tap/k8sutils`

Linux, Windows: download the latest release binary from the [releases](https://github.com/deweysasser/k8sutils/releases) page.

# Examples

List all HPAs in the default namespace:

    $ k8sutils hpa
    NAME                     REFERENCE               CPU  TARGET  MINPODS  MIN%  MAXPODS  REPLICAS  REP%  GRAPHICAL SCALE
    test-1             Deployment/test-1             0%   45%           2    4%       48         2    4%  | X-----------------------------|
    test-2             Deployment/test-2             2%   50%           2    4%       48        12   33%  | ---------X--------------------|
    test-3             Deployment/test-3             4%   60%           5   50%       10         5   50%  |               X---------------|
    test-4             Deployment/test-4             0%   60%           2   20%       10         2   20%  |      X------------------------|
    test-5             Deployment/test-5             0%   60%           2   13%       15         4   26%  |    ---X-----------------------|
   
Force your HPA minimums to 50% of max scale:

    k8sutils hpa --min 50% --all

Double the max of one HPA:

    k8sutils hpa my-hpa --max 2x

Set all minimums to 10 pods:

    k8sutils hpa --min 10 --all

Cut your minimums in half:

    k8sutils hpa --min 0.5x --all

Change CPU scaling for one HPA:

    k8sutils hpa my-hpa --cpu 50

# Usage

## k8sutils hpa

Control HPA min and max scale in human centered unitis like "2x" and "50%"

NOTE:  when setting minimum, % is relative to the current max scale

```
$ ./k8sutils hpa -h
Usage: k8sutils hpa [<hpa-list> ...] [flags]

Arguments:
  [<hpa-list> ...]    Names of specific HPAs to modify

Flags:
  -h, --help                           Show context-sensitive help.
      --version                        Show program version

      --minimum=STRING                 Set minimum to this number
      --maximum=STRING                 Set maximum to this number
      --kubeconfig="~/.kube/config"    Path to the kubeconfig file
  -n, --namespace=STRING               Namespace to modify HPAs in
      --context=STRING                 Context to use in kubeconfig
  -l, --labels=KEY=VALUE;...           Label filters to select HPAs
      --all                            Modify all HPAs in the namespace

Info
  --debug                   Show debugging information
  --dry-run                 Do not modify anything
  --output-format="auto"    How to show program output (auto|terminal|jsonl)
  --quiet                   Be less verbose than usual
```

# Building from source

```
git clone https://github.com/deweysasser/k8sutils
cd k8sutils
make
```

# TODO:
- this should be turned into a kubectl plugin

