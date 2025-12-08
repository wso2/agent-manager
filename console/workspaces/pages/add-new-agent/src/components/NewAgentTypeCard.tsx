import {
  Card,
  CardActionArea,
  CardContent,
  Box,
  Typography,
} from "@wso2/oxygen-ui";

interface NewAgentTypeCardProps {
  type: string;
  title: string;
  subheader: string;
  icon: React.ReactNode;
  content: React.ReactNode;
  onClick: (type: string) => void;
}

export const NewAgentTypeCard = (props: NewAgentTypeCardProps) => {
  const { type, title, subheader, icon, content, onClick } = props;
  const handleClick = () => {
    onClick(type);
  };

  return (
    <Card
      variant="outlined"
      elevation={0}
      sx={{
        width: 450,
        transition: "all 0.3s ease-in-out",
        "&.MuiCard-root": {
          backgroundColor: "background.paper",
        },
        "&:hover": {
          borderColor: "primary.main",
        },
      }}
    >
      <CardActionArea
        onClick={handleClick}
        sx={{
          height: "100%",
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
        }}
      >
        <CardContent
          sx={{
            flexGrow: 1,
            display: "flex",
            flexDirection: "column",
            width: "100%",
            p: 3,
            justifyContent: "center",
            alignItems: "center",
          }}
        >
          <Typography variant="h4" textAlign="center" gutterBottom>
            {title}
          </Typography>

          <Box
            sx={{
              color: "primary.main",
            }}
          >
            {icon}
          </Box>
          <Typography
            variant="body2"
            color="text.secondary"
            textAlign="center"
            sx={{ mb: 2 }}
          >
            {subheader}
          </Typography>
          <Box sx={{ mb: 2 }}>{content}</Box>
        </CardContent>
      </CardActionArea>
    </Card>
  );
};
