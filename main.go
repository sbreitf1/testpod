package main

import "fmt"

func main() {
	tpl, err := ReadTemplate()
	if err != nil {
		fmt.Println(err)
	}

	// export PODNAME="testpod-$(hostname)-$(date '+%s')"
	fmt.Println(MakeManifestFromTemplate("yolo", tpl))

	//TODO clone kubeconfig

	/*
		echo "$PODYAML" | kubectl apply -f -
		kubectl wait --for=condition=ready --timeout=30s pod/$PODNAME
		set +e
		kubectl exec -it $PODNAME -- $PODSHELL
		kubectl delete --wait=false pod $PODNAME
		kubectl delete --wait=false netpol $PODNAME
	*/
}
