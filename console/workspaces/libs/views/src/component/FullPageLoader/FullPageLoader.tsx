import { User as PersonOutlined } from "@wso2/oxygen-ui-icons-react";
import { Box, LinearProgress} from "@wso2/oxygen-ui";

export function FullPageLoader() {    return (
        <Box sx={{
            display: 'flex',
            flexDirection: 'column',
            justifyContent: 'center',
            alignItems: 'center',
            height: '100vh',
            width: '100vw'
        }}>
            <Box sx={{ display: 'flex', flexDirection:'column', justifyContent: 'center', alignItems: 'center', gap: 2 }}>
                <Box sx={{ fontSize: 100, display: 'inline-flex' }}>
                    <PersonOutlined size={100} color="primary" />
                </Box>
                <LinearProgress color="primary" value={50} sx={{ width: '100%' }} />
            </Box>
        </Box>
    );
}
