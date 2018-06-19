package testing

import (
	"fmt"
	"net/http"
	"testing"

	th "github.com/gophercloud/gophercloud/testhelper"
	fakeclient "github.com/gophercloud/gophercloud/testhelper/client"
)

// ValidJSONTemplate is a valid OpenStack Capsule template in JSON format
const ValidJSONTemplate = `
{
    "capsuleVersion": "beta",
    "kind": "capsule",
    "metadata": {
        "labels": {
            "app": "web",
            "app1": "web1"
        },
        "name": "template"
    },
    "restartPolicy": "Always",
    "spec": {
        "containers": [
            {
                "command": [
                    "/bin/bash"
                ],
                "env": {
                    "ENV1": "/usr/local/bin",
                    "ENV2": "/usr/bin"
                },
                "image": "ubuntu",
                "imagePullPolicy": "ifnotpresent",
                "ports": [
                    {
                        "containerPort": 80,
                        "hostPort": 80,
                        "name": "nginx-port",
                        "protocol": "TCP"
                    }
                ],
                "resources": {
                    "requests": {
                        "cpu": 1,
                        "memory": 1024
                    }
                },
                "workDir": "/root"
            }
        ]
    }
}
`

// ValidYAMLTemplate is a valid OpenStack Capsule template in YAML format
const ValidYAMLTemplate = `
capsuleVersion: beta
kind: capsule
metadata:
  name: template
  labels:
    app: web
    app1: web1
restartPolicy: Always
spec:
  containers:
  - image: ubuntu
    command:
      - "/bin/bash"
    imagePullPolicy: ifnotpresent
    workDir: /root
    ports:
      - name: nginx-port
        containerPort: 80
        hostPort: 80
        protocol: TCP
    resources:
      requests:
        cpu: 1
        memory: 1024
    env:
      ENV1: /usr/local/bin
      ENV2: /usr/bin
`

// ValidJSONTemplateParsed is the expected parsed version of ValidJSONTemplate
var ValidJSONTemplateParsed = map[string]interface{}{
	"capsuleVersion": "beta",
	"kind":           "capsule",
	"restartPolicy":  "Always",
	"metadata": map[string]interface{}{
		"name": "template",
		"labels": map[string]string{
			"app":  "web",
			"app1": "web1",
		},
	},
	"spec": map[string]interface{}{
		"containers": []map[string]interface{}{
			map[string]interface{}{
				"image": "ubuntu",
				"command": []interface{}{
					"/bin/bash",
				},
				"imagePullPolicy": "ifnotpresent",
				"workDir":         "/root",
				"ports": []interface{}{
					map[string]interface{}{
						"name":          "nginx-port",
						"containerPort": float64(80),
						"hostPort":      float64(80),
						"protocol":      "TCP",
					},
				},
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{
						"cpu":    float64(1),
						"memory": float64(1024),
					},
				},
				"env": map[string]interface{}{
					"ENV1": "/usr/local/bin",
					"ENV2": "/usr/bin",
				},
			},
		},
	},
}

// ValidYAMLTemplateParsed is the expected parsed version of ValidYAMLTemplate
var ValidYAMLTemplateParsed = map[string]interface{}{
	"capsuleVersion": "beta",
	"kind":           "capsule",
	"restartPolicy":  "Always",
	"metadata": map[string]interface{}{
		"name": "template",
		"labels": map[string]string{
			"app":  "web",
			"app1": "web1",
		},
	},
	"spec": map[interface{}]interface{}{
		"containers": []map[interface{}]interface{}{
			map[interface{}]interface{}{
				"image": "ubuntu",
				"command": []interface{}{
					"/bin/bash",
				},
				"imagePullPolicy": "ifnotpresent",
				"workDir":         "/root",
				"ports": []interface{}{
					map[interface{}]interface{}{
						"name":          "nginx-port",
						"containerPort": 80,
						"hostPort":      80,
						"protocol":      "TCP",
					},
				},
				"resources": map[interface{}]interface{}{
					"requests": map[interface{}]interface{}{
						"cpu":    1,
						"memory": 1024,
					},
				},
				"env": map[interface{}]interface{}{
					"ENV1": "/usr/local/bin",
					"ENV2": "/usr/bin",
				},
			},
		},
	},
}

// HandleCapsuleGetSuccessfully test setup
func HandleCapsuleGetSuccessfully(t *testing.T) {
	th.Mux.HandleFunc("/capsules/cc654059-1a77-47a3-bfcf-715bde5aad9e", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", fakeclient.TokenID)

		w.WriteHeader(http.StatusOK)
		w.Header().Add("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"uuid": "cc654059-1a77-47a3-bfcf-715bde5aad9e",
			"status": "Running",
			"id": 1,
			"user_id": "d33b18c384574fd2a3299447aac285f0",
			"project_id": "6b8ffef2a0ac42ee87887b9cc98bdf68",
			"cpu": 1,
			"memory": "1024M",
			"meta_name": "test",
			"meta_labels": {"web": "app"},
			"created_at": "2018-01-12 09:37:25+00:00",
			"updated_at": "2018-01-12 09:37:26+00:00",
			"links": [
				{
				  "href": "http://10.10.10.10/v1/capsules/cc654059-1a77-47a3-bfcf-715bde5aad9e",
				  "rel": "self"
				},
				{
				  "href": "http://10.10.10.10/capsules/cc654059-1a77-47a3-bfcf-715bde5aad9e",
				  "rel": "bookmark"
				}
			],
			"capsule_version": "beta",
			"restart_policy":  "always",
			"containers_uuids": ["1739e28a-d391-4fd9-93a5-3ba3f29a4c9b"],
			"addresses": {
				"b1295212-64e1-471d-aa01-25ff46f9818d": [
					{
						"version": 4,
						"preserve_on_delete": false,
						"addr": "172.24.4.11",
						"port": "8439060f-381a-4386-a518-33d5a4058636",
						"subnet_id": "4a2bcd64-93ad-4436-9f48-3a7f9b267e0a"
					}
				]
			},
			"volumes_info": {
				"67618d54-dd55-4f7e-91b3-39ffb3ba7f5f": [
					"1739e28a-d391-4fd9-93a5-3ba3f29a4c9b"
				]
			},
			"host": "test-host",
			"status_reason": "No reason",
			"containers": [
				{
					"addresses": {
						"b1295212-64e1-471d-aa01-25ff46f9818d": [
							{
								"version": 4,
								"preserve_on_delete": false,
								"addr": "172.24.4.11",
								"port": "8439060f-381a-4386-a518-33d5a4058636",
								"subnet_id": "4a2bcd64-93ad-4436-9f48-3a7f9b267e0a"
							}
						]
					},
					"image": "test",
					"labels": {"foo": "bar"},
					"created_at": "2018-01-12 09:37:25+00:00",
					"updated_at": "2018-01-12 09:37:26+00:00",
					"workdir": "/root",
					"disk": 0,
					"id": 1,
					"security_groups": ["default"],
					"image_pull_policy": "ifnotpresent",
					"task_state": "Creating",
					"user_id": "d33b18c384574fd2a3299447aac285f0",
					"project_id": "6b8ffef2a0ac42ee87887b9cc98bdf68",
					"uuid": "1739e28a-d391-4fd9-93a5-3ba3f29a4c9b",
					"hostname": "test-hostname",
					"environment": {"USER1": "test"},
					"memory": "1024M",
					"status": "Running",
					"auto_remove": false,
					"container_id": "5109ebe2ca595777e994416208bd681b561b25ce493c34a234a1b68457cb53fb",
					"websocket_url": "ws://10.10.10.10/",
					"auto_heal": false,
					"host": "test-host",
					"image_driver": "docker",
					"status_detail": "Just created",
					"status_reason": "No reason",
					"websocket_token": "2ba16a5a-552f-422f-b511-bd786102691f",
					"name": "test-demo-omicron-13",
					"restart_policy": {
						"MaximumRetryCount": "0",
						"Name": "always"
					},
					"ports": [80],
					"meta": {"key1": "value1"},
					"command": "testcmd",
					"capsule_id": 1,
					"runtime": "runc",
					"cpu": 1,
					"interactive": true
				}
			]
		}`)
	})
}

const CapsuleListBody = `
{
	"capsules": [
		{
			"uuid": "cc654059-1a77-47a3-bfcf-715bde5aad9e",
			"status": "Running",
			"id": 1,
			"user_id": "d33b18c384574fd2a3299447aac285f0",
			"project_id": "6b8ffef2a0ac42ee87887b9cc98bdf68",
			"cpu": 1,
			"memory": "1024M",
			"meta_name": "test",
			"meta_labels": {"web": "app"},
			"created_at": "2018-01-12 09:37:25+00:00",
			"updated_at": "2018-01-12 09:37:25+01:00",
			"links": [
				{
				  "href": "http://10.10.10.10/v1/capsules/cc654059-1a77-47a3-bfcf-715bde5aad9e",
				  "rel": "self"
				},
				{
				  "href": "http://10.10.10.10/capsules/cc654059-1a77-47a3-bfcf-715bde5aad9e",
				  "rel": "bookmark"
				}
			],
			"capsule_version": "beta",
			"restart_policy":  "always",
			"containers_uuids": ["1739e28a-d391-4fd9-93a5-3ba3f29a4c9b", "d1469e8d-bcbc-43fc-b163-8b9b6a740930"],
			"addresses": {
				"b1295212-64e1-471d-aa01-25ff46f9818d": [
					{
						"version": 4,
						"preserve_on_delete": false,
						"addr": "172.24.4.11",
						"port": "8439060f-381a-4386-a518-33d5a4058636",
						"subnet_id": "4a2bcd64-93ad-4436-9f48-3a7f9b267e0a"
					}
				]
			},
			"volumes_info": {
				"67618d54-dd55-4f7e-91b3-39ffb3ba7f5f": [
					"4b725a92-2197-497b-b6b1-fb8caa4cb99b"
				]
			},
			"host": "test-host",
			"status_reason": "No reason"
		}
	]
}`

// HandleCapsuleCreateSuccessfully creates an HTTP handler at `/capsules` on the test handler mux
// that responds with a `Create` response.
func HandleCapsuleCreateSuccessfully(t *testing.T) {
	th.Mux.HandleFunc("/capsules", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "POST")
		th.TestHeader(t, r, "X-Auth-Token", fakeclient.TokenID)
		th.TestHeader(t, r, "Accept", "application/json")
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintf(w, `{}`)
	})
}

// HandleCapsuleListSuccessfully test setup
func HandleCapsuleListSuccessfully(t *testing.T) {
	th.Mux.HandleFunc("/capsules/", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "GET")
		th.TestHeader(t, r, "X-Auth-Token", fakeclient.TokenID)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, CapsuleListBody)
	})
}

func HandleCapsuleDeleteSuccessfully(t *testing.T) {
	th.Mux.HandleFunc("/capsules/963a239d-3946-452b-be5a-055eab65a421", func(w http.ResponseWriter, r *http.Request) {
		th.TestMethod(t, r, "DELETE")
		th.TestHeader(t, r, "X-Auth-Token", fakeclient.TokenID)
		w.WriteHeader(http.StatusNoContent)
	})
}
