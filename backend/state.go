package main

import (
	"errors"
)

type State int

const (
	INITIATED State = iota
	PROCESSING
	CANCELLED
	SUCCESS
	FAILED
)

func (s State) String() string {
	switch s {
	case INITIATED:
		return "INITIATED"
	case PROCESSING:
		return "PROCESSING"
	case CANCELLED:
		return "CANCELLED"
	case SUCCESS:
		return "SUCCESS"
	case FAILED:
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}

var INVALID_STATE_CHANGE_REQUEST = errors.New("invalid state change request")

// INITIATED -> processing,CANCELLED
// PROCESSING -> SUCCESS,CANCELLED,FAILED
// FAILED -> PROCESSING

var status = make(map[string]int)

func SetState(id string, changestate State) (bool, error) {
	currentState := status[id]
	if currentState == 0 && changestate == INITIATED {
		status[id] = int(changestate)
		return true, nil
	}

	switch currentState {
	case int(INITIATED):
		switch changestate {
		case PROCESSING, CANCELLED:
			break
		default:
			return false, INVALID_STATE_CHANGE_REQUEST
		}
	case int(PROCESSING):
		switch changestate {
		case SUCCESS, CANCELLED, FAILED:
			break
		default:
			return false, INVALID_STATE_CHANGE_REQUEST
		}
	case int(FAILED):
		switch changestate {
		case PROCESSING:
			break
		default:
			return false, INVALID_STATE_CHANGE_REQUEST
		}
	case int(CANCELLED):
		return false, INVALID_STATE_CHANGE_REQUEST
	default:
		return false, INVALID_STATE_CHANGE_REQUEST
	}
	status[id] = int(changestate)
	return true, nil
}

func GetState(id string) State {
	return State(status[id])
}
