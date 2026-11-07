// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/suxinwl/GoSuxin/framework/encoding/gjson"
)

// Issue2746 is the golang structure for table issue2746.
type Issue2746 struct {
	Id       uint        `json:"ID"       orm:"id"       ` // User ID
	Nickname string      `json:"NICKNAME" orm:"nickname" ` // User Nickname
	Tag      *gjson.Json `json:"TAG"      orm:"tag"      ` //
	Info     string      `json:"INFO"     orm:"info"     ` //
	Tag2     *gjson.Json `json:"TAG_2"    orm:"tag2"     ` // Tag2
}
