import { AdapterDateFns, Collapse, DatePickers, Form, TextField } from "@wso2/oxygen-ui";
import { History, Timer } from "@wso2/oxygen-ui-icons-react";
import { useState } from "react";

export function CreateMonitorForm() {
    // TempState
    const [ds, setDs] = useState("previous");
    return (
        <Form.Stack>
            <Form.Section>
                <Form.Header   >
                    Basic Details
                </Form.Header>
                <Form.ElementWrapper name="name" label="Name" >
                    <TextField name="name" placeholder="Enter monitor name" required fullWidth />
                </Form.ElementWrapper>
                <Form.ElementWrapper name="description" label="Description" >
                    <TextField name="description" placeholder="Enter monitor description" fullWidth multiline minRows={3} />
                </Form.ElementWrapper>
            </Form.Section>
            <Form.Section>
                <Form.Header>
                    Data Collection
                </Form.Header>
                <Form.Stack direction="row">
                    <Form.CardButton onClick={() => setDs("previous")} selected={ds === "previous"}>
                        <Form.CardHeader title={<Form.Stack direction="row" spacing={1} justifyContent="center" alignItems="center">
                            <History size={24} />
                            <Form.Body>Past Traces</Form.Body>
                        </Form.Stack>} />
                    </Form.CardButton>
                    <Form.CardButton onClick={() => setDs("future")} selected={ds === "future"}>
                        <Form.CardHeader title={<Form.Stack direction="row" spacing={1} justifyContent="center" alignItems="center">
                            <Timer size={24} />
                            <Form.Body>Future Traces</Form.Body>
                        </Form.Stack>} />
                    </Form.CardButton>
                </Form.Stack>
                <Collapse in={ds === "previous"}>
                    <Form.Stack direction="row" >
                        <Form.ElementWrapper name="startTime" label="Start Time" >
                            <DatePickers.LocalizationProvider dateAdapter={AdapterDateFns}>
                                <DatePickers.DateTimePicker />
                            </DatePickers.LocalizationProvider>
                        </Form.ElementWrapper>
                        <Form.ElementWrapper name="endTime" label="End Time" >
                            <DatePickers.LocalizationProvider dateAdapter={AdapterDateFns}>
                                <DatePickers.DateTimePicker />
                            </DatePickers.LocalizationProvider>
                        </Form.ElementWrapper>
                    </Form.Stack>
                </Collapse>
            </Form.Section>
        </Form.Stack>

    )
}
