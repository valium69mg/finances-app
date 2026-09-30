package domain

import "errors"

// ErrTransferLegLocked is returned when a savings movement that belongs to a
// transfer is edited: the two legs must keep mirroring each other, so a leg can
// only be removed (which removes both), never changed on its own.
var ErrTransferLegLocked = errors.New("transfer legs cannot be edited: delete the transfer and record it again")
