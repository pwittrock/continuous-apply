// Copyright 2018 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package firestore

// DatabaseRootPath returns the path for the database root resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s/databases/%s", project, database)
// instead.
func DatabaseRootPath(project, database string) string {
	return "" +
		"projects/" +
		project +
		"/databases/" +
		database +
		""
}

// DocumentRootPath returns the path for the document root resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s/databases/%s/documents", project, database)
// instead.
func DocumentRootPath(project, database string) string {
	return "" +
		"projects/" +
		project +
		"/databases/" +
		database +
		"/documents" +
		""
}

// DocumentPathPath returns the path for the document path resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s/databases/%s/documents/%s", project, database, documentPath)
// instead.
func DocumentPathPath(project, database, documentPath string) string {
	return "" +
		"projects/" +
		project +
		"/databases/" +
		database +
		"/documents/" +
		documentPath +
		""
}

// AnyPathPath returns the path for the any path resource.
//
// Deprecated: Use
//   fmt.Sprintf("projects/%s/databases/%s/documents/%s/%s", project, database, document, anyPath)
// instead.
func AnyPathPath(project, database, document, anyPath string) string {
	return "" +
		"projects/" +
		project +
		"/databases/" +
		database +
		"/documents/" +
		document +
		"/" +
		anyPath +
		""
}
