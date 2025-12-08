import { ImageList } from "./imageList";

export interface ImageProps {
    src: ImageList;
    alt: string;
    width?: number | string;
    height?: number | string;
    style?: React.CSSProperties;
}
export default function Image({ src, alt, width = "100%", height = "100%", style }: ImageProps) {
    return <img src={src} alt={alt} width={width} height={height} style={style} />;
}
