package program

import (
	"context"
	"errors"
	"fmt"
	"github.com/rs/zerolog/log"
	v1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"math"
	"regexp"
	"strconv"
)

type Hpa struct {
	Minimum    string            `aliases:"min" help:"Set minimum to this number"`
	Maximum    string            `aliases:"max" help:"Set maximum to this number"`
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

		return errors.New("no HPAs selected")
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

		context := config.Contexts[config.CurrentContext]
		if context.Namespace == "" {
			panic("Namespace is not set in the current context and no namespace flag provided")
		}
		program.Namespace = context.Namespace
	}

	ctx := context.WithValue(context.Background(), "options", options)

	// Get HPAs
	hpas, err := program.getHpas(err, clientset, ctx)
	if err != nil {
		return err
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
