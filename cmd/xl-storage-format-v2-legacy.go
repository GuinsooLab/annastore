// Copyright (c) 2022 GuinsooLab
//
// This file is part of GuinsooLab stack
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package cmd

import (
	"fmt"

	"github.com/tinylib/msgp/msgp"
)

// unmarshalV unmarshals with a specific header version.
func (x *xlMetaV2VersionHeader) unmarshalV(v uint8, bts []byte) (o []byte, err error) {
	switch v {
	case 1:
		return x.unmarshalV1(bts)
	case xlHeaderVersion:
		return x.UnmarshalMsg(bts)
	}
	return bts, fmt.Errorf("unknown xlHeaderVersion: %d", v)
}

// unmarshalV1 decodes version 1, never released.
func (x *xlMetaV2VersionHeader) unmarshalV1(bts []byte) (o []byte, err error) {
	var zb0001 uint32
	zb0001, bts, err = msgp.ReadArrayHeaderBytes(bts)
	if err != nil {
		err = msgp.WrapError(err)
		return
	}
	if zb0001 != 4 {
		err = msgp.ArrayError{Wanted: 4, Got: zb0001}
		return
	}
	bts, err = msgp.ReadExactBytes(bts, (x.VersionID)[:])
	if err != nil {
		err = msgp.WrapError(err, "VersionID")
		return
	}
	x.ModTime, bts, err = msgp.ReadInt64Bytes(bts)
	if err != nil {
		err = msgp.WrapError(err, "ModTime")
		return
	}
	{
		var zb0002 uint8
		zb0002, bts, err = msgp.ReadUint8Bytes(bts)
		if err != nil {
			err = msgp.WrapError(err, "Type")
			return
		}
		x.Type = VersionType(zb0002)
	}
	{
		var zb0003 uint8
		zb0003, bts, err = msgp.ReadUint8Bytes(bts)
		if err != nil {
			err = msgp.WrapError(err, "Flags")
			return
		}
		x.Flags = xlFlags(zb0003)
	}
	o = bts
	return
}

// unmarshalV unmarshals with a specific metadata version.
func (j *xlMetaV2Version) unmarshalV(v uint8, bts []byte) (o []byte, err error) {
	switch v {
	// We accept un-set as latest version.
	case 0, xlMetaVersion:
		// Clear omitempty fields:
		if j.ObjectV2 != nil && len(j.ObjectV2.PartIndices) > 0 {
			j.ObjectV2.PartIndices = j.ObjectV2.PartIndices[:0]
		}
		o, err = j.UnmarshalMsg(bts)

		// Clean up PartEtags on v1
		if j.ObjectV2 != nil {
			allEmpty := true
			for _, tag := range j.ObjectV2.PartETags {
				if len(tag) != 0 {
					allEmpty = false
					break
				}
			}
			if allEmpty {
				j.ObjectV2.PartETags = nil
			}
		}
		return o, err
	}
	return bts, fmt.Errorf("unknown xlMetaVersion: %d", v)
}
