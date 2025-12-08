export interface AppRoute {
    path: string;
    wildPath?: string;
    index?: boolean;
    children: {[key: string]: AppRoute};
}

export interface GenaratedRoute {
    path?: string;
    children: {[key: string]: GenaratedRoute};
}
