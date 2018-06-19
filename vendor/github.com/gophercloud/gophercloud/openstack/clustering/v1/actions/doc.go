/*
Package actions provides listing and retrieving of senlin actions for the OpenStack Clustering Service.

Example to list actions

	opts := actions.ListOpts{
		Limit: 5,
	}

	err = actions.List(serviceClient, opts).EachPage(func(page pagination.Page) (bool, error) {
		actionInfos, err := actions.ExtractActions(page)
		if err != nil {
			return false, err
		}

		for _, actionInfo := range actionInfos {
			fmt.Println("%+v\n", actionInfo)
		}
		return true, nil
	})

Example to get an action

	action, err := actions.Get(serviceClient, "edce3528-864f-41fb-8759-f4707925cc09").Extract()
	if err != nil {
		panic(err)
	}
	fmt.Printf("Action %+v: ", action)

*/
package actions
