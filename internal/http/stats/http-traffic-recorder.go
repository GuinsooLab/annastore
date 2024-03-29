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

package stats

import (
	"io"
	"net/http"
)

// IncomingTrafficMeter counts the incoming bytes from the underlying request.Body.
type IncomingTrafficMeter struct {
	countBytes int64
	io.ReadCloser
}

// Read calls the underlying Read and counts the transferred bytes.
func (r *IncomingTrafficMeter) Read(p []byte) (n int, err error) {
	n, err = r.ReadCloser.Read(p)
	r.countBytes += int64(n)

	return n, err
}

// BytesRead returns the number of transferred bytes
func (r *IncomingTrafficMeter) BytesRead() int64 {
	return r.countBytes
}

// OutgoingTrafficMeter counts the outgoing bytes through the responseWriter.
type OutgoingTrafficMeter struct {
	countBytes int64
	// wrapper for underlying http.ResponseWriter.
	http.ResponseWriter
}

// Write calls the underlying write and counts the output bytes
func (w *OutgoingTrafficMeter) Write(p []byte) (n int, err error) {
	n, err = w.ResponseWriter.Write(p)
	w.countBytes += int64(n)
	return n, err
}

// Flush calls the underlying Flush.
func (w *OutgoingTrafficMeter) Flush() {
	w.ResponseWriter.(http.Flusher).Flush()
}

// BytesWritten returns the number of transferred bytes
func (w *OutgoingTrafficMeter) BytesWritten() int64 {
	return w.countBytes
}
