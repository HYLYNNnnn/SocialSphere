import { useEffect, useMemo, useRef, useState } from "react";
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
import { getImageConfig, generateImage, generatedImageFile, uploadPost, MAX_IMAGE_PROMPT_CHARACTERS } from "../api";

const Overlay = styled.div`position: fixed; inset: 0; background-color: rgba(0,0,0,.5); display: flex; justify-content: center; align-items: center; z-index: 1400;`;
const MainContainer = styled.div`background-color: #27272a; min-height: 100vh; padding-bottom: 32px;`;
const HeaderContainer = styled.div`display: flex; flex-direction: column; align-items: center;`;
const settingsControlStyle = { boxSizing: "border-box", width: "100%", minHeight: 40, marginTop: 6,
  padding: "8px 10px", border: "1px solid #71717a", borderRadius: 6, background: "#27272a", color: "white", font: "inherit" };

function preferredModel(config, source) {
  const preferred = source === "personal" ? "gpt-image-2" : config.default_model;
  return config.models.find((model) => model.id === preferred)?.id
    || config.models.find((model) => model.id === config.default_model)?.id || config.models[0].id;
}

export default function Landing() {
  const [index, setIndex] = useState(-1);
  const [inputValue, setInputValue] = useState("");
  const [generating, setGenerating] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [generated, setGenerated] = useState(null);
  const [imageConfig, setImageConfig] = useState(null);
  const [configError, setConfigError] = useState("");
  const [configAttempt, setConfigAttempt] = useState(0);
  const [keySource, setKeySource] = useState("");
  const [model, setModel] = useState("");
  // Personal keys live only in this component; navigation or refresh discards them.
  const [apiKey, setApiKey] = useState("");
  const pending = useRef(false);
  useEffect(() => {
    const controller = new AbortController();
    let active = true;
    setImageConfig(null); setConfigError("");
    getImageConfig({ signal: controller.signal }).then((config) => {
      if (!active) return;
      const source = config.server_configured ? "server" : config.user_keys_allowed ? "personal" : "";
      setImageConfig(config); setKeySource(source); setModel(preferredModel(config, source));
    }).catch((error) => {
      if (active && error.name !== "AbortError") setConfigError("Image settings could not be loaded. Please try again.");
    });
    return () => { active = false; controller.abort(); };
  }, [configAttempt]);
  const promptLength = Array.from(inputValue.trim()).length;
  const promptTooLong = promptLength > MAX_IMAGE_PROMPT_CHARACTERS;
  const sourceAvailable = !!imageConfig && (keySource === "server" ? imageConfig.server_configured
    : keySource === "personal" && imageConfig.user_keys_allowed);
  const canGenerate = sourceAvailable && imageConfig.models.some((item) => item.id === model)
    && (keySource !== "personal" || !!apiKey.trim());
  const changeSource = (event) => {
    const source = event.target.value;
    setKeySource(source); setModel(preferredModel(imageConfig, source));
    if (source !== "personal") setApiKey("");
  };
  const photos = useMemo(() => generated ? [{ src: generated.imageUrl, width: 1024, height: 1024, alt: generated.prompt }] : [], [generated]);
  const create = async (event) => {
    event.preventDefault();
    if (pending.current || !inputValue.trim() || promptTooLong || !canGenerate) return;
    pending.current = true; setGenerating(true);
    try { setGenerated(await generateImage(inputValue, { model, keySource, ...(keySource === "personal" ? { apiKey } : {}) })); }
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
        <IconButton type="submit" aria-label="Generate image" disabled={!inputValue.trim() || promptTooLong || !canGenerate || generating || uploading}><ArrowForwardIcon /></IconButton>
      </Paper>
      <Typography id="image-description-limit" variant="caption" sx={{ color: promptTooLong ? "error.light" : "#CCCCCC", mb: 2 }}>
        {`${promptLength} / ${MAX_IMAGE_PROMPT_CHARACTERS} characters${promptTooLong ? " — shorten your description." : ""}`}
      </Typography>
      <details open style={{ boxSizing: "border-box", width: "80%", maxWidth: 600, marginBottom: 48,
        border: "1px solid #52525b", borderRadius: 10, padding: 16, color: "white", background: "#333337" }}>
        <summary style={{ cursor: "pointer", fontWeight: 600 }}>Image settings</summary>
        {!imageConfig && !configError && <p role="status">Loading image options…</p>}
        {configError && <div role="alert"><p>{configError}</p>
          <button type="button" onClick={() => setConfigAttempt((attempt) => attempt + 1)}>Try again</button></div>}
        {imageConfig && !imageConfig.server_configured && !imageConfig.user_keys_allowed
          && <p role="status">Image creation is not available on this site right now. You can still upload and share your own photos in Collection.</p>}
        {imageConfig && (imageConfig.server_configured || imageConfig.user_keys_allowed) && <>
          <label style={{ display: "block", marginTop: 16 }}>API key source
            <select aria-label="API key source" value={keySource} onChange={changeSource} disabled={generating || uploading} style={settingsControlStyle}>
              {imageConfig.server_configured && <option value="server">Use SocialSphere’s key</option>}
              {imageConfig.user_keys_allowed && <option value="personal">Use my API key</option>}
            </select>
          </label>
          {keySource === "personal" && <div style={{ marginTop: 16 }}>
            <label style={{ display: "block" }}>OpenAI API key
              <input aria-label="OpenAI API key" type="password" autoComplete="off" spellCheck={false}
                value={apiKey} onChange={(event) => setApiKey(event.target.value)} disabled={generating || uploading}
                placeholder="Enter your API key" style={settingsControlStyle} />
            </label>
            <button type="button" onClick={() => setApiKey("")} disabled={!apiKey || generating || uploading}
              style={{ marginTop: 8, padding: "6px 12px", cursor: "pointer" }}>Clear key</button>
            <p style={{ fontSize: 13, lineHeight: 1.5, color: "#e4e4e7" }}>
              Your key stays in this page’s memory until you clear it or leave. Only when you select Generate, it is sent through this SocialSphere server to OpenAI; this app does not store it. Usage is charged to your OpenAI API account.
              {" "}<a href="https://platform.openai.com/api-keys" target="_blank" rel="noreferrer" style={{ color: "#93c5fd" }}>Manage OpenAI API keys</a>
            </p>
            {!apiKey.trim() && <p style={{ fontSize: 13, color: "#fcd34d" }}>Enter your API key to enable image creation.</p>}
          </div>}
          <label style={{ display: "block", marginTop: 16 }}>Image model
            <select aria-label="Image model" value={model} onChange={(event) => setModel(event.target.value)}
              disabled={generating || uploading} style={settingsControlStyle}>
              {imageConfig.models.map((item) => <option key={item.id} value={item.id}>{item.label}</option>)}
            </select>
          </label>
        </>}
      </details>
    </HeaderContainer>
    <PhotoAlbum photos={photos} layout="rows" targetRowHeight={200} onClick={({ index }) => setIndex(index)} />
    <Lightbox slides={photos} open={index >= 0} index={index} close={() => { if (!uploading) setIndex(-1); }}
      plugins={[Fullscreen, Slideshow, Thumbnails, Zoom]} toolbar={{ buttons: [<IconButton key="publish" type="button" aria-label="Publish generated image"
        disabled={uploading || !generated} onClick={publish}><FileUploadRoundedIcon sx={{ color: "#CCCCCC" }} /></IconButton>, "close"] }} />
  </MainContainer>;
}
