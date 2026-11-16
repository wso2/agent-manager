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
  Box,
  Typography,
  Form,
} from "@wso2/oxygen-ui";

interface NewAgentTypeCardProps {
  type: string;
  title: string;
  subheader: string;
  icon: React.ReactNode;
  ctaLabel: string;
  onClick: (type: string) => void;
}

export const NewAgentTypeCard = (props: NewAgentTypeCardProps) => {
  const { type, title, subheader, icon, ctaLabel, onClick } = props;
  const handleClick = () => {
    onClick(type);
  };

  return (
    <Form.CardButton
      onClick={handleClick}
      sx={{
        width: { xs: "100%", sm: 450 },
        py: 2,
        // The CTA pill below is purely visual (a real <button> can't nest inside
        // this CardButton's own <button>), so it fills in on hover/focus of the
        // whole card, not on its own hover — clicking anywhere still does the same thing.
        "&:hover .cta-pill, &:focus-visible .cta-pill": {
          bgcolor: "primary.main",
          color: "primary.contrastText",
        },
      }}
    >
      <Typography width="100%" variant="h4" textAlign="center">
        {title}
      </Typography>
      <Form.CardContent>
        <Box
          sx={{
            display: "flex",
            justifyContent: "center",
            alignItems: "flex-end",
            height: 180,
            mb: 2,
          }}
        >
          {icon}
        </Box>
        <Typography variant="body2" color="text.primary" textAlign="center">{subheader}</Typography>
        <Box display="flex" justifyContent="center" mt={3}>
          <Box
            component="span"
            className="cta-pill"
            sx={{
              display: "inline-flex",
              alignItems: "center",
              justifyContent: "center",
              px: 3,
              py: 1,
              borderRadius: 999,
              border: "1px solid",
              borderColor: "primary.main",
              color: "primary.main",
              fontWeight: 700,
              whiteSpace: "nowrap",
              transition: "background-color 0.15s ease, color 0.15s ease",
            }}
          >
            {ctaLabel}
          </Box>
        </Box>
      </Form.CardContent>
    </Form.CardButton>
  );
};
