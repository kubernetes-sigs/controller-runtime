package internal

import (
	"bytes"
	"html/template"
)

// RenderTemplate renders single string template
func RenderTemplate(tmplStr string, data interface{}) (rendered string, err error) {
	renderedArray, err := RenderTemplates([]string{tmplStr}, data)
	if err != nil {
		return "", err
	}
	return renderedArray[0], nil
}

// RenderTemplates returns an []string to render the templates
func RenderTemplates(argTemplates []string, data interface{}) (args []string, err error) {
	var t *template.Template

	for _, arg := range argTemplates {
		t, err = template.New(arg).Parse(arg)
		if err != nil {
			args = nil
			return
		}

		buf := &bytes.Buffer{}
		err = t.Execute(buf, data)
		if err != nil {
			args = nil
			return
		}
		args = append(args, buf.String())
	}

	return
}
