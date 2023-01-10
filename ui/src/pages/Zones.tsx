import { useState, useEffect, Fragment } from "react";
import { useQuery, } from "react-query";
import {
    Datagrid,
    List,
    TextField,
    SimpleForm,
    TextInput,
    ReferenceField,
    ReferenceArrayField,
    Create,
    Show,
    SimpleShowLayout,
    BooleanField,
    BooleanInput,
    CreateButton,
    ExportButton,
    TopToolbar,
    useDataProvider,
    BulkDeleteButton,
    BulkExportButton,
} from 'react-admin';

const ZoneCreateButton = () => {
    const [disableCreate, setDisableCreate] = useState(false);
    const dataProvider = useDataProvider();

    useEffect(() => {
        dataProvider.getFlag("multi-zone")
            .then((response: any) => {
                setDisableCreate(response.json["multi-zone"] != true)
            });
    });

    return (
        <CreateButton disabled={disableCreate}/>
    );
};

const ListActions = () => (
    <TopToolbar>
        <ZoneCreateButton/>
        <ExportButton/>
    </TopToolbar>
);

const ZoneBulkDeleteButton = () => {
    const [disableCreate, setDisableCreate] = useState(false);
    const dataProvider = useDataProvider();

    useEffect(() => {
        dataProvider.getFlag("multi-zone")
            .then((response: any) => {
                setDisableCreate(response.json["multi-zone"] != true)
            });
    });

    return (
        <BulkDeleteButton disabled={disableCreate}/>
    );
};

const ZoneListBulkActions = () => (
    <Fragment>
        <BulkExportButton />
        <ZoneBulkDeleteButton />
    </Fragment>
);

export const ZoneList = () => (
    <List actions={<ListActions/>} bulkActionButtons={<ZoneListBulkActions/>}>
        <Datagrid rowClick="show" bulkActionButtons={false}>
            <TextField label="Name" source="name" />
            <TextField label="Description" source="description" />
            <TextField label="CIDR" source="cidr" />
        </Datagrid>
    </List>
)

export const ZoneShow = () => (
    <Show>
        <SimpleShowLayout>
            <TextField label="ID" source="id" />
            <TextField label="Name" source="name" />
            <TextField label="Description" source="description" />
            <TextField label="CIDR" source="cidr" />
            <BooleanField label="Hub Zone" source="hub_zone" />
            <ReferenceArrayField label="Peers" source="peers" reference="peers">
                <Datagrid rowClick="show" bulkActionButtons={false} >
                    <ReferenceField label="ID" source="id" reference="peers" link="show"/>
                    <ReferenceField label="Device" source="device_id" reference='devices' link="show"/>
                    <TextField label="Endpoint IP" source="endpoint_ip" />
                    <TextField label="Node Address" source="node_address" />
                    <BooleanField label="Hub Router" source="hub_router" />
                </Datagrid>
            </ReferenceArrayField>
        </SimpleShowLayout>
    </Show>
)

export const ZoneCreate = () => (
    <Create>
        <SimpleForm>
            <TextInput label="Name" source="name" />
            <TextInput label="Description" source="description" />
            <TextInput label="CIDR" source="cidr" />
            <BooleanInput label="Hub Zone" source="hub_zone" />
        </SimpleForm>
    </Create>
);
