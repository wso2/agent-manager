import {
    Store as BusinessSharp,
    Circle as CircleOutlined, 
    Circle as CircleRounded,
    AlertTriangle as WarningRounded,
} from "@wso2/oxygen-ui-icons-react";
import { Box, Button, ButtonGroup, CircularProgress, useTheme } from "@wso2/oxygen-ui";
import { Link } from "react-router-dom";

export enum TabStatus {
    ACTIVE = "active",
    INACTIVE = "not-deployed",
    DEPLOYING = "in-progress",
    ERROR = "error",
}

export interface LinkTabProps {
  to: string;
  label: string;
  id: string;
  status: TabStatus;
  isProduction: boolean;
}


export const getTabIcon = (status: TabStatus, isSelected: boolean) => {
    switch (status) {
        case TabStatus.ACTIVE:
            return <CircleRounded color={"success"} />;
        case TabStatus.ERROR:
            return <WarningRounded color={isSelected ? "inherit" : "error"} />;
        case TabStatus.DEPLOYING:
            return <CircularProgress color={isSelected ? "inherit" : "warning"} />;
        default: // INACTIVE
            return <CircleOutlined color={isSelected ? "inherit" : "disabled"} />;
    }
}

const getTabEndIcon = (isProduction: boolean) => {
    switch (isProduction) {
        case true:
            return <BusinessSharp color="inherit" />;
        default:
            return undefined;
    }
}
export function TopNavBarTab(props: LinkTabProps & { selectedId?: string }) {
    const { to, label, status, id, isProduction, selectedId } = props;
    useTheme();
     
    const isSelected = selectedId ? id === selectedId : false;
    return (
        <Button 
            component={Link} 
            to={to} 
            startIcon={getTabIcon(status,isSelected)} 
            endIcon={getTabEndIcon(isProduction)}
            variant={isSelected ? "contained" : "text"}
        >
            {label}
        </Button>
    );
}

export function TopNavBarGroup(props: { tabs: LinkTabProps[]; selectedId?: string }) {
    const { tabs, selectedId } = props;
    return (
        <Box>
            <ButtonGroup
                variant="text"
                color="inherit"
                orientation="horizontal"
                size="small"
                aria-label="vertical outlined button group"
            >
                {tabs.map((prop) => (
                    <TopNavBarTab key={prop.id} {...prop} selectedId={selectedId} />
                ))}
            </ButtonGroup>
        </Box>
    );
}
