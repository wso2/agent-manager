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

import {
  Card,
  CardContent,
  Box,
  Typography,
  CardHeader,
} from "@wso2/oxygen-ui";

interface NewAgentTypeCardProps {
  type: string;
  title: string;
  subheader: string;
  icon: React.ReactNode;
  onClick: (type: string) => void;
}

export const NewAgentTypeCard = (props: NewAgentTypeCardProps) => {
  const { type, title, subheader, icon, onClick } = props;
  const handleClick = () => {
    onClick(type);
  };

  return (
    <Card
      variant="outlined"
      elevation={0}
      sx={{
        width: 450,
        py: 2,
        transition: "all 0.3s ease-in-out",
        cursor: "pointer",
        "&.MuiCard-root": {
          backgroundColor: "background.default",
          "&:hover": {
            borderColor: "primary.main",
            boxShadow: theme => theme.shadows[2],
          },
        },
      }}
      onClick={handleClick}
    >
      <CardHeader title={
        <Typography variant="h4" textAlign="center">
            {title}
        </Typography>
      } />
      <CardContent>
        <Box
          sx={{
            display: "flex",
            justifyContent: "center",
            alignItems: "flex-end",
            height: 250,
            mb: 10,
          }}
        >
          {icon}
        </Box>
        <Typography variant="body2" textAlign="center">{subheader}</Typography>
      </CardContent>
    </Card>
  );
};
