package mygo

import winapi "golang.org/x/sys/windows"

// knownFolder is the Windows known folder of a user's folder, "" when the
// system has none.
func knownFolder(name PathName) string {
	id := map[PathName]*winapi.KNOWNFOLDERID{
		PathDesktop: winapi.FOLDERID_Desktop, PathDocuments: winapi.FOLDERID_Documents,
		PathDownloads: winapi.FOLDERID_Downloads, PathMusic: winapi.FOLDERID_Music,
		PathPictures: winapi.FOLDERID_Pictures, PathVideos: winapi.FOLDERID_Videos,
	}[name]
	if id == nil {
		return ""
	}
	dir, err := winapi.KnownFolderPath(id, winapi.KF_FLAG_DEFAULT)
	if err != nil {
		return ""
	}
	return dir
}
