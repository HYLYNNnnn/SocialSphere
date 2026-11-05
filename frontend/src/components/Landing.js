import { useMemo, useRef, useState } from "react";
import PhotoAlbum from "react-photo-album";
import Lightbox from "yet-another-react-lightbox";
import Fullscreen from "yet-another-react-lightbox/plugins/fullscreen";
import Slideshow from "yet-another-react-lightbox/plugins/slideshow";
import Thumbnails from "yet-another-react-lightbox/plugins/thumbnails";
import Zoom from "yet-another-react-lightbox/plugins/zoom";
import styled from "styled-components";
import { Typography, Paper, InputBase, IconButton, CircularProgress } from "@mui/material";
import ArrowForwardIcon from "@mui/icons-material/ArrowForward";
import FileUploadRoundedIcon from "@mui/icons-material/FileUploadRounded";
import { message } from "antd";
import { generateImage, generatedImageFile, uploadPost, MAX_IMAGE_PROMPT_CHARACTERS } from "../api";

const Overlay = styled.div`position: fixed; inset: 0; background-color: rgba(0,0,0,.5); display: flex; justify-content: center; align-items: center; z-index: 1400;`;
const MainContainer = styled.div`background-color: #27272a; min-height: 100vh; padding-bottom: 32px;`;
const HeaderContainer = styled.div`display: flex; flex-direction: column; align-items: center;`;

export default function Landing() {
  const [index, setIndex] = useState(-1);
  const [inputValue, setInputValue] = useState("");
  const [generating, setGenerating] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [generated, setGenerated] = useState(null);
  const pending = useRef(false);
  const promptLength = Array.from(inputValue.trim()).length;
  const promptTooLong = promptLength > MAX_IMAGE_PROMPT_CHARACTERS;
  const photos = useMemo(() => generated ? [{ src: generated.imageUrl, width: 1024, height: 1024, alt: generated.prompt }] : [], [generated]);
  const create = async (event) => {
    event.preventDefault();
    if (pending.current || !inputValue.trim() || promptTooLong) return;
    pending.current = true; setGenerating(true);
    try { setGenerated(await generateImage(inputValue)); }
    catch (error) { message.error(error.message); }
    finally { pending.current = false; setGenerating(false); }
  };
  const publish = async () => {
    if (pending.current || !generated) return;
    pending.current = true; setUploading(true);
    try {
      await uploadPost(generatedImageFile(generated.imageUrl), Array.from(generated.prompt).slice(0, 2000).join(""));
      message.success("Your generated image was posted."); setIndex(-1);
    } catch (error) { message.error(error.message); }
    finally { pending.current = false; setUploading(false); }
  };
  return <MainContainer>
    {(generating || uploading) && <Overlay role="status" aria-label={generating ? "Generating image" : "Publishing image"}><CircularProgress color="info" size={80} /></Overlay>}
    <HeaderContainer>
      <Typography variant="h1" sx={{ fontSize: { xs: "3rem", sm: "5.2rem" }, mt: "128px", color: "white" }}>SocialSphere</Typography>
      <Typography variant="h5" sx={{ fontSize: "1.2rem", color: "white", m: "0 20px", textAlign: "center" }}>Create images with AI and share your moments.</Typography>
      <Paper component="form" onSubmit={create} sx={{ p: "2px 4px", display: "flex", alignItems: "center", width: "80%", maxWidth: 600, borderRadius: "10px", mt: 4, mb: 1 }}>
        <InputBase multiline sx={{ ml: 1, flex: 1 }} placeholder="Describe the image you want to create…"
          inputProps={{ "aria-label": "Image description", "aria-describedby": "image-description-limit" }} value={inputValue} onChange={(event) => setInputValue(event.target.value)} disabled={generating || uploading} />
        <IconButton type="submit" aria-label="Generate image" disabled={!inputValue.trim() || promptTooLong || generating || uploading}><ArrowForwardIcon /></IconButton>
      </Paper>
      <Typography id="image-description-limit" variant="caption" sx={{ color: promptTooLong ? "error.light" : "#CCCCCC", mb: 8 }}>
        {`${promptLength} / ${MAX_IMAGE_PROMPT_CHARACTERS} characters${promptTooLong ? " — shorten your description." : ""}`}
      </Typography>
    </HeaderContainer>
    <PhotoAlbum photos={photos} layout="rows" targetRowHeight={200} onClick={({ index }) => setIndex(index)} />
    <Lightbox slides={photos} open={index >= 0} index={index} close={() => { if (!uploading) setIndex(-1); }}
      plugins={[Fullscreen, Slideshow, Thumbnails, Zoom]} toolbar={{ buttons: [<IconButton key="publish" type="button" aria-label="Publish generated image"
        disabled={uploading || !generated} onClick={publish}><FileUploadRoundedIcon sx={{ color: "#CCCCCC" }} /></IconButton>, "close"] }} />
  </MainContainer>;
}
