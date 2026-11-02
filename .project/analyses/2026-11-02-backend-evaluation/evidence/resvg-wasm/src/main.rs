use std::io::{Read, Write};

fn main() {
    let args: Vec<String> = std::env::args().collect();
    let max_width: u32 = args.get(1).and_then(|s| s.parse().ok()).unwrap_or(800);
    let mut data = Vec::new();
    std::io::stdin().read_to_end(&mut data).expect("stdin");
    let mut opt = resvg::usvg::Options::default();
    opt.fontdb_mut().load_fonts_dir("/fonts");
    opt.resources_dir = None;
    let tree = match resvg::usvg::Tree::from_data(&data, &opt) {
        Ok(t) => t,
        Err(e) => { eprintln!("parse error: {e}"); std::process::exit(2) }
    };
    let size = tree.size();
    let scale = if size.width() > max_width as f32 { max_width as f32 / size.width() } else { 1.0 };
    let w = (size.width() * scale).ceil() as u32;
    let h = (size.height() * scale).ceil() as u32;
    let mut pixmap = resvg::tiny_skia::Pixmap::new(w.max(1), h.max(1)).expect("pixmap");
    resvg::render(&tree, resvg::tiny_skia::Transform::from_scale(scale, scale), &mut pixmap.as_mut());
    let png = pixmap.encode_png().expect("png");
    std::io::stdout().write_all(&png).expect("stdout");
}
