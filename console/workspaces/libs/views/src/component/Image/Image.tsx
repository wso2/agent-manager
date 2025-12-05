/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

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
