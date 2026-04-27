package controllers

// OGXServer CRD permissions
//+kubebuilder:rbac:groups=ogx.io,resources=ogxservers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ogx.io,resources=ogxservers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ogx.io,resources=ogxservers/finalizers,verbs=update

// Deployment permissions - controller creates and manages deployments
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

// Service permissions - controller creates and manages services
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

// TRANSITIONAL: Legacy resource access for annotation-driven adoption.
// These permissions will be removed when adoption support is deprecated.
//+kubebuilder:rbac:groups="",resources=pods,verbs=list

// ServiceAccount permissions - controller creates and manages service accounts for PVC permissions
//+kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete

//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch

// RoleBinding permissions - controller creates and manages role bindings for PVC permissions
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete

//+kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=use
//+kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,resourceNames=anyuid,verbs=use

//+kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update

// ConfigMap permissions - controller reads user configmaps and manages operator config configmaps
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch

// NetworkPolicy permissions - controller creates and manages network policies
//+kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete

// Ingress permissions - controller creates and manages ingresses for external access
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

// PodDisruptionBudget permissions - controller creates and manages voluntary disruption controls
//+kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete

// HorizontalPodAutoscaler permissions - controller creates and manages HPAs for server pods
//+kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
