#!/usr/bin/env bash

set -e

# Source common functions
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/utils.sh"

# Default provider
PROVIDER="ollama"
MODEL=""
ARGS=""
ENV_VARS=""

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --provider)
            PROVIDER="$2"
            shift 2
            ;;
        --model)
            MODEL="$2"
            shift 2
            ;;
        --runtime-args)
            ARGS="$2"
            shift 2
            ;;
        --runtime-env)
            ENV_VARS="$2"
            shift 2
            ;;
        --help)
            echo "Usage: $0 [OPTIONS]"
            echo "Options:"
            echo "  --provider NAME     Provider to use (if not set default: ollama)"
            echo "                      Supported providers: ollama, vllm"
            echo "  --model NAME        Model to run (if not set, default: llama3.2:1b)"
            echo "                      For vllm see all https://docs.vllm.ai/en/latest/models/supported_models.html"
            echo "  --runtime-args ARGS         Additional arguments for the inference server, can include model port host etc"
            echo "  --runtime-env KEY=VALUE     Environment variables (comma-separated, e.g KEY1=VAL1,KEY2=VAL2)"
            echo "  --help             Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

load_provider_config "${PROVIDER}" "${MODEL}" "${ENV_VARS}"

NAMESPACE=$(get_namespace "${PROVIDER}")
SERVER_NAME=$(get_server_name "${PROVIDER}")
VOLUMN=$(get_volume_name "${PROVIDER}")

# Use provided args or default one, this is only for container not initContainer
if [ -z "${ARGS}" ]; then
    DEPLOYMENT_ARGS="${DEFAULT_ARGS}"
else
    DEPLOYMENT_ARGS="${ARGS}"
fi

echo "Checking if namespace ${NAMESPACE} exists..."
if ! kubectl get namespace "${NAMESPACE}" &> /dev/null; then
    echo "Creating namespace ${NAMESPACE}..."
    kubectl create namespace "${NAMESPACE}"
else
    echo "Namespace ${NAMESPACE} already exists"
fi
ENV_YAML=$(convert_env_to_yaml "${ENV_VARS}" "${NAMESPACE}")

echo "Start deploying ${PROVIDER} as provider with configuration:"
echo "  ServingRuntime Image: ${IMAGE}"
echo "  Inference Server: ${INFERENCE_SERVER}"
echo "  Model: ${MODEL}"
echo ""

# Generate SCC related config if needed based on provider
generate_security_context "${PROVIDER}" "${NAMESPACE}"

echo "Creating ${SERVER_NAME} deployment and service with image: ${IMAGE}..."
cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${SERVER_NAME}
  namespace: ${NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: ${SERVER_NAME}
  template:
    metadata:
      labels:
        app: ${SERVER_NAME}
${OPENSHIFT_ANNOTATION}
    spec:
      initContainers:
        - name: ${SERVER_NAME}-downloadmodel
          command: ${COMMAND}
          imagePullPolicy: Always
          volumeMounts:
            - name: ${VOLUMN}
              mountPath: /root/.${PROVIDER}
          terminationMessagePolicy: File
          image: ${IMAGE}
          args: ["${INIT_ARGS}"]
${SERVICE_ACCOUNT:+      serviceAccountName: ${SERVICE_ACCOUNT}}
${SECURITY_CONTEXT_YAML}
      containers:
        - name: ${SERVER_NAME}
          image: ${IMAGE}
          command: ${COMMAND}
          args: ["${DEPLOYMENT_ARGS}"]
          env:
${ENV_YAML}
          ports:
          - containerPort: ${PORT}
          resources:
            requests:
              cpu: "500m"
              memory: "1Gi"
          readinessProbe:
            httpGet:
              path: ${HEALTH_PATH}
              port: ${PORT}
            initialDelaySeconds: 30
            periodSeconds: 30
            timeoutSeconds: 5
            failureThreshold: 3
            successThreshold: 1
          ${CONTAINER_SECURITY_CONTEXT_YAML}
          volumeMounts:
          - name: ${VOLUMN}
            mountPath: /root/.${PROVIDER}
      volumes:
        - name: ${VOLUMN}
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: ${SERVER_NAME}-service
  namespace: ${NAMESPACE}
spec:
  selector:
    app: ${SERVER_NAME}
  ports:
  - protocol: TCP
    port: ${PORT}
    targetPort: ${PORT}
  type: ClusterIP
EOF

echo "This may take up to 5 minutes for the image to be pulled and container to start..."
if ! kubectl rollout status "deployment/${SERVER_NAME}" -n "${NAMESPACE}" --timeout=300s; then
    echo "Error: Deployment failed to become ready within 5 minutes"
    exit 1
fi
echo "Deployment is ready!"

echo ""
echo "${PROVIDER} inference server is now running!"
echo "   Namespace: ${NAMESPACE}"
echo "   Service: ${SERVER_NAME}-service"
echo "   Port: ${PORT}"
echo ""
echo "Access at:"
echo "   http://${SERVER_NAME}-service.${NAMESPACE}.svc.cluster.local:${PORT}"
