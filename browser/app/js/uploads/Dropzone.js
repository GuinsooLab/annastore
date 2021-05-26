/*
 * MinIO Object Storage (c) 2021 MinIO, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import React from "react"
import { connect } from "react-redux"
import ReactDropzone from "react-dropzone"
import * as actions from "./actions"

// Dropzone is a drag-and-drop element for uploading files. It will create a
// landing zone of sorts that automatically receives the files.
export class Dropzone extends React.Component {
  onDrop(files) {
    const { uploadFile } = this.props
    // FIXME: Currently you can upload multiple files, but only one abort
    // modal will be shown, and progress updates will only occur for one
    // file at a time. See #171.
    files.forEach(file => {
      uploadFile(file)
    })
  }

  render() {
    // Overwrite the default styling from react-dropzone; otherwise it
    // won't handle child elements correctly.
    const style = {
      flex: "1",
      borderWidth: "0",
      borderStyle: "dashed",
      borderColor: "#fff"
    }
    const activeStyle = {
      borderWidth: "2px",
      borderColor: "#777"
    }
    const rejectStyle = {
      backgroundColor: "#ffdddd"
    }
    const getStyle = (isDragActive, isDragAccept, isDragReject) => ({
      ...style,
      ...(isDragActive ? activeStyle : {}),
      ...(isDragReject ? rejectStyle : {})
    })

    // disableClick means that it won't trigger a file upload box when
    // the user clicks on a file.
    return (
      <ReactDropzone
        onDrop={this.onDrop.bind(this)}
      >
        {({getRootProps, getInputProps, isDragActive, isDragAccept, isDragReject}) => (
          <div
            {...getRootProps({
              onClick: event => event.stopPropagation()
            })}
            style={getStyle(isDragActive, isDragAccept, isDragReject)}
          >
            <input {...getInputProps()} />
            {this.props.children}
          </div>
        )}
      </ReactDropzone>
    )
  }
}

const mapDispatchToProps = dispatch => {
  return {
    uploadFile: file => dispatch(actions.uploadFile(file))
  }
}

export default connect(undefined, mapDispatchToProps)(Dropzone)
