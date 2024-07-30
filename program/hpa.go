package program

import (
	"context"
	"errors"
	"fmt"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/rs/zerolog/log"
	v1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Hpa struct {
	Minimum    string            `aliases:"min" help:"Set minimum to this number"`
	Maximum    string            `aliases:"max" help:"Set maximum to this number"`
	CPUTarget  int               `aliases:"cpu" help:"Set scaling target"`
	Info       bool              `help:"Show information about the HPAs"`
	Kubeconfig string            `help:"Path to the kubeconfig file" type:"path" default:"~/.kube/config"`
	Namespace  string            `short:"n" help:"Namespace to modify HPAs in"`
	Context    string            `help:"Context to use in kubeconfig"`
	Labels     map[string]string `short:"l" help:"Label filters to select HPAs"`
	All        bool              `help:"Modify all HPAs in the namespace"`
	HPAList    []string          `arg:"" optional:"" help:"Names of specific HPAs to modify"`
}

type strategy func(hpa *v1.HorizontalPodAutoscaler) error

func (program *Hpa) Run(options *Options) error {

	if !program.All && len(program.HPAList) == 0 && len(program.Labels) == 0 {
		program.Info = true
	}

	// Set up Kubernetes client
	config, err := clientcmd.BuildConfigFromFlags("", program.Kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// Get namespace from context if not provided
	if program.Namespace == "" {
		config, err := clientcmd.LoadFromFile(program.Kubeconfig)
		if err != nil {
			panic(err.Error())
		}

		k8sContext := config.Contexts[config.CurrentContext]
		if k8sContext.Namespace == "" {
			panic("Namespace is not set in the current context and no namespace flag provided")
		}
		program.Namespace = k8sContext.Namespace
	}

	ctx := context.WithValue(context.Background(), "options", options)

	// Get HPAs
	hpas, err := program.getHpas(err, clientset, ctx)
	if err != nil {
		return err
	}

	if program.Info {
		// NAME                      REFERENCE                            TARGETS   MINPODS   MAXPODS   REPLICAS   AGE
		// Example:
		// test-hpa                  Deployment/test                      26%/45%   4         100       9          60d

		program.printHPAs(hpas)
		return nil
	}

	cal, err := program.getStrategy()

	if err != nil {
		return err
	}

	var listErrors []error

	for _, hpa := range hpas {
		err := modifyHPA(ctx, &hpa,
			cal,
			clientset, program.Namespace)

		listErrors = append(listErrors, err)

		if err != nil {
			fmt.Printf("Failed to update HPA %s: %v\n", hpa.Name, err)

		}
	}

	return errors.Join(listErrors...)
}

func (program *Hpa) printHPAs(hpas []v1.HorizontalPodAutoscaler) {
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleLight)
	t.Style().Options.DrawBorder = false
	t.Style().Options.SeparateRows = false
	t.Style().Options.SeparateColumns = false
	t.Style().Options.SeparateHeader = false

	t.AppendHeader(table.Row{"NAME", "REFERENCE", "CPU", "TARGET", "MINPODS", "MIN%", "MAXPODS", "REPLICAS", "REP%", "Graphical Scale"})
	for _, hpa := range hpas {
		cpu := "?"
		if hpa.Status.CurrentCPUUtilizationPercentage != nil {
			cpu = fmt.Sprint(*hpa.Status.CurrentCPUUtilizationPercentage, "%")
		}
		t.AppendRow(table.Row{
			hpa.Name,
			hpa.Spec.ScaleTargetRef.Kind + "/" + hpa.Spec.ScaleTargetRef.Name,
			cpu,
			fmt.Sprint(*hpa.Spec.TargetCPUUtilizationPercentage, "%"),
			*hpa.Spec.MinReplicas,
			fmt.Sprintf("%3d%%",
				int(float64(*hpa.Spec.MinReplicas)/float64(hpa.Spec.MaxReplicas)*100)),
			hpa.Spec.MaxReplicas,
			hpa.Status.CurrentReplicas,
			fmt.Sprintf("%3d%%",
				int(float64(hpa.Status.CurrentReplicas)/float64(hpa.Spec.MaxReplicas)*100)),
			formatGraphicalPercentage(hpa.Status.CurrentReplicas, *hpa.Spec.MinReplicas, hpa.Spec.MaxReplicas),
		})

	}
	t.Render()
}

var field = strings.Repeat("-", 30)
var spaces = strings.Repeat(" ", len(field))

// formatGraphicalPercentage draws a text representation of the percentage, like >   |----X----|<
func formatGraphicalPercentage(current int32, min int32, max int32) string {

	scale := float64(len(field))
	leading := float64(min) / float64(max)
	mark := float64(current) / float64(max)

	ls := int(leading * scale)
	ms := int(mark*scale) - ls
	ts := int(scale) - ms - ls

	return "|" +
		spaces[0:ls] +
		field[0:ms] +
		"X" +
		field[0:ts] +
		"|"

}

func (program *Hpa) getHpas(err error, clientset *kubernetes.Clientset, ctx context.Context) ([]v1.HorizontalPodAutoscaler, error) {

	var hpas []v1.HorizontalPodAutoscaler

	if len(program.HPAList) > 0 {
		for _, hpaName := range program.HPAList {
			hpa, err := clientset.AutoscalingV1().HorizontalPodAutoscalers(program.Namespace).Get(context.TODO(), hpaName, metav1.GetOptions{})
			if err != nil {
				fmt.Printf("Failed to get HPA %s: %v\n", hpaName, err)
				continue
			}
			hpas = append(hpas, *hpa)
		}
	} else {
		listOptions := metav1.ListOptions{}
		if len(program.Labels) > 0 {
			labelSelector := ""
			for key, value := range program.Labels {
				if labelSelector != "" {
					labelSelector += ","
				}
				labelSelector += fmt.Sprintf("%s=%s", key, value)
			}
			listOptions.LabelSelector = labelSelector
		}

		hpaList, err := clientset.AutoscalingV1().HorizontalPodAutoscalers(program.Namespace).List(context.TODO(), listOptions)
		if err != nil {
			return hpas, err
		}

		hpas = hpaList.Items
	}

	return hpas, nil
}

var (
	Number     = regexp.MustCompile(`^[0-9]+$`)
	Percentage = regexp.MustCompile(`^[0-9\\.]+%$`)
	Multiply   = regexp.MustCompile(`^[0-9\\.]+x$`)
)

func (program *Hpa) getStrategy() (strategy, error) {

	switch {
	case program.CPUTarget > 0:
		return func(hpa *v1.HorizontalPodAutoscaler) error {
			*hpa.Spec.TargetCPUUtilizationPercentage = int32(program.CPUTarget)
			return nil
		}, nil

	case Number.MatchString(program.Minimum):
		if num, err := strconv.Atoi(program.Minimum); err != nil {
			return nil, err
		} else {
			return func(hpa *v1.HorizontalPodAutoscaler) error {
				minimum := int32(num)
				hpa.Spec.MinReplicas = &minimum
				reconcileMax(hpa)
				return nil
			}, nil
		}
	case Percentage.MatchString(program.Minimum):
		if percent, err := strconv.ParseFloat(program.Minimum[:len(program.Minimum)-1], 32); err != nil {
			return nil, err
		} else {
			return func(hpa *v1.HorizontalPodAutoscaler) error {
				minimum := int32(math.Ceil(percent / 100 * float64(hpa.Spec.MaxReplicas)))
				*hpa.Spec.MinReplicas = minimum
				reconcileMax(hpa)
				return nil
			}, nil
		}

	case Multiply.MatchString(program.Minimum):
		if multiplier, err := strconv.ParseFloat(program.Minimum[:len(program.Minimum)-1], 32); err != nil {
			return nil, err
		} else {
			return func(hpa *v1.HorizontalPodAutoscaler) error {
				minimum := int32(float64(*hpa.Spec.MinReplicas) * multiplier)
				hpa.Spec.MinReplicas = &minimum
				reconcileMax(hpa)
				return nil
			}, nil
		}

	case Number.MatchString(program.Maximum):
		if num, err := strconv.Atoi(program.Maximum); err != nil {
			return nil, err
		} else {
			return func(hpa *v1.HorizontalPodAutoscaler) error {
				hpa.Spec.MaxReplicas = int32(num)
				reconcileMin(hpa)
				return nil
			}, nil
		}

	case Percentage.MatchString(program.Maximum):
		if percent, err := strconv.ParseFloat(program.Maximum[:len(program.Maximum)-1], 32); err != nil {
			return nil, err
		} else {
			return func(hpa *v1.HorizontalPodAutoscaler) error {
				maximum := int32(math.Ceil(float64(percent) / 100 * float64(hpa.Spec.MaxReplicas)))
				hpa.Spec.MaxReplicas = maximum
				reconcileMin(hpa)
				return nil
			}, nil
		}
	case Multiply.MatchString(program.Maximum):
		if multiplier, err := strconv.ParseFloat(program.Maximum[:len(program.Maximum)-1], 32); err != nil {
			return nil, err
		} else {
			return func(hpa *v1.HorizontalPodAutoscaler) error {
				maximum := int32(float64(hpa.Spec.MaxReplicas) * multiplier)
				hpa.Spec.MaxReplicas = maximum
				reconcileMin(hpa)
				return nil
			}, nil
		}
	default:
		return nil, errors.New("invalid arguments")
	}
}

func reconcileMax(hpa *v1.HorizontalPodAutoscaler) {
	if *hpa.Spec.MinReplicas > hpa.Spec.MaxReplicas {
		hpa.Spec.MaxReplicas = *hpa.Spec.MinReplicas
	}
}

func reconcileMin(hpa *v1.HorizontalPodAutoscaler) {
	if *hpa.Spec.MinReplicas > hpa.Spec.MaxReplicas {
		*hpa.Spec.MinReplicas = hpa.Spec.MaxReplicas
	}
}

// modifyHPA modifies the HPA per the strategy function passed
func modifyHPA(ctx context.Context, hpa *v1.HorizontalPodAutoscaler, update strategy, clientset *kubernetes.Clientset, namespace string) error {
	oldMax := hpa.Spec.MaxReplicas
	oldMin := *hpa.Spec.MinReplicas

	if err := update(hpa); err != nil {
		return err
	}

	options := ctx.Value("options").(*Options)

	log.Info().
		Str("from", fmt.Sprint(oldMin, "/", oldMax)).
		Str("to", fmt.Sprint(*hpa.Spec.MinReplicas, "/", hpa.Spec.MaxReplicas)).
		Str("hpa", hpa.Name).
		Msg("Updating HPA")

	if !options.DryRun {
		log.Debug().Msg("Updating via API")
		_, err := clientset.AutoscalingV1().HorizontalPodAutoscalers(namespace).Update(ctx, hpa, metav1.UpdateOptions{})

		if err != nil {
			return err
		} else {
			log.Debug().Msg("Updated")
		}
	}

	return nil
}
