
# k8sutils

A series of command line utils that allow me to manipulate the cluster as I want

# TODO:  this should be turned into a kubectl plugin

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