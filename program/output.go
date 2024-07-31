package program

import (
	"fmt"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/rs/zerolog/log"
	"math"
	"os"
	"sort"
	"strings"

	v1 "k8s.io/api/autoscaling/v1"
)

var field = strings.Repeat(".", 40)
var spaces = strings.Repeat(" ", len(field))

func initColors(options *Options) {

	if options.OutputFormat == "terminal" ||
		(options.OutputFormat == "auto" && isTerminal(os.Stdout)) {
		log.Debug().Msg("Enabling colors")
	} else {
		log.Debug().Msg("Disabling colors")
		text.DisableColors()
	}
}

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

type Mark struct {
	// Character to use as the marker
	Mark string
	// Position of the mark between 0 and max
	Postion int
}

var (
	Max = Mark{"|", math.MaxInt}
)

// formatGraphicalPercentage draws a text representation of the percentage, like >   |----X----|<
func formatMarks(min, max int32, marks ...Mark) string {

	sort.Slice(marks, func(i, j int) bool {
		return marks[i].Postion < marks[j].Postion
	})

	builder := strings.Builder{}

	builder.WriteString("|")

	scale := float64(len(field))
	leading := float64(min) / float64(max)

	ls := int(leading * scale)

	builder.WriteString(spaces[0:ls])

	cur := ls

	for _, mark := range marks {
		pos := int(float64(mark.Postion) / float64(max) * scale)

		if mark.Postion > int(max) {
			continue
		}

		chars := pos - cur

		if chars > 0 {
			builder.WriteString(field[0 : chars-1])
			builder.WriteString(mark.Mark)

			cur = cur + chars + len(mark.Mark)
		} else if mark == marks[0] {
			builder.WriteString(mark.Mark)
			cur = cur + len(mark.Mark)
		}
	}

	chars := int(scale) - builder.Len()

	builder.WriteString(field[0:chars])
	builder.WriteString("| ")
	if marks[len(marks)-1] == Max {
		builder.WriteString(fmt.Sprint(max))
	}

	return builder.String()
}

func (program *Hpa) printHPAs(hpas []v1.HorizontalPodAutoscaler) {
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleLight)
	t.Style().Options.DrawBorder = false
	t.Style().Options.SeparateRows = false
	t.Style().Options.SeparateColumns = false
	t.Style().Options.SeparateHeader = false

	t.AppendHeader(table.Row{"NAME", "REFERENCE", "CPU", "SCALE"})
	for _, hpa := range hpas {
		cpu := "unknown"
		if hpa.Status.CurrentCPUUtilizationPercentage != nil && hpa.Spec.TargetCPUUtilizationPercentage != nil {
			cpu = formatMarks(0, 100,
				Mark{fmt.Sprint(*hpa.Status.CurrentCPUUtilizationPercentage, "%"), int(*hpa.Status.CurrentCPUUtilizationPercentage)},
				Mark{"<", int(*hpa.Spec.TargetCPUUtilizationPercentage)},
			)

			log.Debug().
				Int32("current", *hpa.Status.CurrentCPUUtilizationPercentage).
				Int32("target", *hpa.Spec.TargetCPUUtilizationPercentage).
				Msg("cpu")
			if *hpa.Status.CurrentCPUUtilizationPercentage <= *hpa.Spec.TargetCPUUtilizationPercentage {
				cpu = text.FgGreen.Sprint(cpu)
			} else if *hpa.Status.CurrentCPUUtilizationPercentage >= 90 {
				cpu = text.FgRed.Sprint(cpu)
			} else {
				cpu = text.FgYellow.Sprint(cpu)
			}
		}

		pods := formatMarks(*hpa.Spec.MinReplicas, hpa.Spec.MaxReplicas,
			Mark{fmt.Sprint(hpa.Status.CurrentReplicas), int(hpa.Status.CurrentReplicas)},
			Mark{"|", int(hpa.Status.DesiredReplicas)},
			Max,
		)

		podColor := text.FgGreen

		switch {
		case hpa.Status.CurrentReplicas > int32(float32(hpa.Spec.MaxReplicas)*.8):
			podColor = text.FgYellow
		case hpa.Status.CurrentReplicas > int32(float32(hpa.Spec.MaxReplicas)*.8):
			podColor = text.FgYellow
		case hpa.Status.CurrentReplicas >= hpa.Spec.MaxReplicas:
			podColor = text.FgMagenta
		}

		pods = podColor.Sprint(pods)

		t.AppendRow(table.Row{
			hpa.Name,
			hpa.Spec.ScaleTargetRef.Kind + "/" + hpa.Spec.ScaleTargetRef.Name,
			cpu,
			pods,
		})

	}
	t.Render()
}
