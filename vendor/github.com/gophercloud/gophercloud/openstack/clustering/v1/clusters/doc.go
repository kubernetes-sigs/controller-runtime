/*
Package clusters provides information and interaction with the clusters through
the OpenStack Clustering service.

Example to Create a cluster

	createOpts := clusters.CreateOpts{
		Name:            "test-cluster",
		DesiredCapacity: 1,
		ProfileUUID:     "b7b870ee-d3c5-4a93-b9d7-846c53b2c2da",
	}

	cluster, err := clusters.Create(serviceClient, createOpts).Extract()
	if err != nil {
		panic(err)
	}

Example to Get Clusters

	clusterName := "cluster123"
	cluster, err := clusters.Get(serviceClient, clusterName).Extract()
	if err != nil {
		panic(err)
	}
	fmt.Printf("%+v\n", cluster)

Example to List Clusters

	listOpts := clusters.ListOpts{
		Name: "testcluster",
	}

	allPages, err := clusters.ListDetail(serviceClient, listOpts).AllPages()
	if err != nil {
		panic(err)
	}

	allClusters, err := clusters.ExtractClusters(allPages)
	if err != nil {
		panic(err)
	}

	for _, cluster := range allClusters {
		fmt.Printf("%+v\n", cluster)
	}

*/
package clusters
