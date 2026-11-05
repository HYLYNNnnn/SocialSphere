import React, { useEffect, useState } from "react";
import { message } from "antd";
import PhotoAlbum from "react-photo-album";
import Lightbox from "yet-another-react-lightbox";
import Fullscreen from "yet-another-react-lightbox/plugins/fullscreen";
import Slideshow from "yet-another-react-lightbox/plugins/slideshow";
import Thumbnails from "yet-another-react-lightbox/plugins/thumbnails";
import Zoom from "yet-another-react-lightbox/plugins/zoom";
import Captions from "yet-another-react-lightbox/plugins/captions";
import "yet-another-react-lightbox/plugins/captions.css";
import DeleteIcon from "@mui/icons-material/Delete";
import IconButton from "@mui/material/IconButton";
import { deletePost } from "../api";

export default function PhotoGallery({ images, username, onDeleted }) {
  const [index, setIndex] = useState(-1);
  const [deleting, setDeleting] = useState(false);
  const photos = images.map((image) => ({ ...image, width: 1024, height: 1024,
    alt: image.caption, title: image.user, description: image.caption }));
  const current = photos[index];
  useEffect(() => { if (index >= images.length) setIndex(-1); }, [index, images.length]);
  const remove = async () => {
    if (deleting || !current || current.user !== username || !window.confirm("Delete this post?")) return;
    setDeleting(true);
    try { await deletePost(current.postId); onDeleted(current.postId); setIndex(-1); message.success("Post deleted."); }
    catch (error) { message.error(error.message); }
    finally { setDeleting(false); }
  };
  return <div className="photo-gallery">
    <PhotoAlbum photos={photos} layout="rows" targetRowHeight={200} onClick={({ index }) => setIndex(index)} />
    <Lightbox slides={photos} open={index >= 0 && !!current} index={index} close={() => { if (!deleting) setIndex(-1); }}
      on={{ view: ({ index: next }) => setIndex(next) }}
      plugins={[Fullscreen, Slideshow, Thumbnails, Zoom, Captions]}
      toolbar={{ buttons: [current?.user === username ? <IconButton key="delete" type="button" aria-label="Delete this post"
        onClick={remove} disabled={deleting}><DeleteIcon sx={{ color: "#CCCCCC" }} /></IconButton> : null, "close"] }} />
  </div>;
}
