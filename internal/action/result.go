package action

func IsSuccess(result *Result) bool {
	return IsContinue(result) || IsReturn(result)
}

func IsContinue(result *Result) bool {
	return result == nil
}

func IsReturn(result *Result) bool {
	if result == nil {
		return false
	}
	return !IsError(result) && !IsRequeue(result)
}

func IsError(result *Result) bool {
	if result == nil {
		return false
	}
	return result.Err != nil
}

func IsRequeue(result *Result) bool {
	if result == nil {
		return false
	}
	if result.Err != nil {
		return false
	}
	return result.Result.Requeue || result.Result.RequeueAfter > 0 //nolint: staticcheck // We have to handle it until it is removed
}
