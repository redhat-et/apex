import { Fragment } from "react";
import { 
    Datagrid,
    List,
    TextField,
    ReferenceArrayField,
    Show,
    SimpleShowLayout,
    SingleFieldList,
    ChipField,
    ReferenceField,
    BulkExportButton,
    BulkDeleteButton,
} from 'react-admin';

const UserListBulkActions = () => (
    <Fragment>
        <BulkExportButton />
        <BulkDeleteButton />
    </Fragment>
);

export const UserList = () => (
    <List>
        <Datagrid rowClick="show" bulkActionButtons={<UserListBulkActions/>}>
            <TextField label = "Username" source="username" />
            <ReferenceField source="zone_id" reference="zones" />
            <ReferenceArrayField label="Devices" source="devices" reference="devices">
                <SingleFieldList linkType="show">
                    <ChipField source="hostname" />
                </SingleFieldList>
            </ReferenceArrayField>
        </Datagrid>
    </List>
);

export const UserShow = () => (
    <Show>
        <SimpleShowLayout>
            <TextField label = "ID" source="id" />
            <TextField label = "Username" source="username" />
            <ReferenceField label = "Zone" source="zone_id" reference="zones" />
            <ReferenceArrayField label="Devices" source="devices" reference="devices">
                <Datagrid rowClick="show" bulkActionButtons={false}>
                    <TextField label="Hostname" source="hostname" />
                    <TextField label="ID" source="id" />
                </Datagrid>
            </ReferenceArrayField>
        </SimpleShowLayout>
    </Show>
);
