import { Fragment } from "react";
import {
    Datagrid,
    List,
    ReferenceField,
    Show,
    SimpleShowLayout,
    TextField,
    ReferenceInput,
    useRecordContext,
    useGetOne,
    Loading,
    BulkExportButton,
    BulkDeleteButton,
} from 'react-admin';

const peerFilters =  [
    <ReferenceInput source="device_id" label="Device" reference="devices" />,
]

const PeerListBulkActions = () => (
    <Fragment>
        <BulkExportButton />
        <BulkDeleteButton />
    </Fragment>
);

export const PeerList = () => (
    <List filters={peerFilters} bulkActionButtons={<PeerListBulkActions/>}>
        <Datagrid rowClick="show" bulkActionButtons={false} >
            <ReferenceField label="Device" source="device_id" reference='devices' link="show"/>
            <ReferenceField label="Zone" source="zone_id" reference='zones' link="show" />
            <TextField label="Endpoint IP" source="endpoint_ip" />
            <TextField label="Allowed IPs" source="allowed_ips" />
            <TextField label="Node Address" source="node_address" />
            <TextField label="STUN Address" source="reflexive_ip4" />
            <TextField label="Child Prefix" source="child_prefix" />
        </Datagrid>
    </List>
);

const PeerTitle = () => {
    const peer = useRecordContext();
    if (!peer) return null;
    const { data: zone, isLoading, error } = useGetOne('zones', { id: peer.zone_id });
    const { data: device, isLoading: isLoading2, error: error2 } = useGetOne('devices', { id: peer.device_id });
    if (isLoading || isLoading2) { return <Loading />; }
    if (error || error2) { return <p>ERROR</p>; }
    return <div>Peer: Device {device.hostname} to Zone {zone.name}</div>;
};

export const PeerShow= () => (
    <Show title={<PeerTitle />}>
        <SimpleShowLayout>
            <TextField label="ID" source="id" />
            <ReferenceField label="Device" source="device_id" reference='devices' link="show" />
            <ReferenceField label="Zone" source="zone_id" reference='zones' link="show" />
            <TextField label="Endpoint IP" source="endpoint_ip" />
            <TextField label="Allowed IPs" source="allowed_ips" />
            <TextField label="Node Address" source="node_address" />
            <TextField label="STUN Address" source="reflexive_ip4" />
            <TextField label="Child Prefix" source="child_prefix" />
        </SimpleShowLayout>
    </Show>
);
