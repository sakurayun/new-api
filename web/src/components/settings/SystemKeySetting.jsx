import React, { useState, useEffect, useCallback } from 'react';
import {
    Table,
    Button,
    Modal,
    Toast,
    Tag,
    Space,
    Typography,
    Input,
    Tabs,
    TabPane,
    Select,
    Popconfirm,
    Banner,
} from '@douyinfe/semi-ui';
import { IconCopy, IconPlus, IconDelete, IconEyeOpened, IconRefresh } from '@douyinfe/semi-icons';
import { API, showError } from '../../helpers';
import { useTranslation } from 'react-i18next';

const { Text } = Typography;

function SystemKeySetting() {
    const { t } = useTranslation();
    const [keys, setKeys] = useState([]);
    const [total, setTotal] = useState(0);
    const [loading, setLoading] = useState(false);
    const [page, setPage] = useState(1);
    const [pageSize] = useState(20);

    const [createVisible, setCreateVisible] = useState(false);
    const [createForm, setCreateForm] = useState({ name: '', expired_time: -1, remark: '' });

    const [editVisible, setEditVisible] = useState(false);
    const [editForm, setEditForm] = useState({});

    const [logs, setLogs] = useState([]);
    const [logTotal, setLogTotal] = useState(0);
    const [logLoading, setLogLoading] = useState(false);
    const [logPage, setLogPage] = useState(1);
    const [logFilter, setLogFilter] = useState({ system_key_id: '', action: '' });

    const [activeTab, setActiveTab] = useState('keys');

    const loadKeys = useCallback(async () => {
        setLoading(true);
        try {
            const res = await API.get('/api/system-key/?page=' + page + '&page_size=' + pageSize);
            const { success, data, message } = res.data;
            if (success) {
                setKeys(data.keys || []);
                setTotal(data.total || 0);
            } else {
                showError(message);
            }
        } catch (e) {
            showError(e.message);
        }
        setLoading(false);
    }, [page, pageSize]);

    const loadLogs = useCallback(async () => {
        setLogLoading(true);
        try {
            let url = '/api/system-key/logs?page=' + logPage + '&page_size=20';
            if (logFilter.system_key_id) url += '&system_key_id=' + logFilter.system_key_id;
            if (logFilter.action) url += '&action=' + logFilter.action;
            const res = await API.get(url);
            const { success, data, message } = res.data;
            if (success) {
                setLogs(data.logs || []);
                setLogTotal(data.total || 0);
            } else {
                showError(message);
            }
        } catch (e) {
            showError(e.message);
        }
        setLogLoading(false);
    }, [logPage, logFilter]);

    useEffect(() => {
        if (activeTab === 'keys') loadKeys();
    }, [loadKeys, activeTab]);

    useEffect(() => {
        if (activeTab === 'logs') loadLogs();
    }, [loadLogs, activeTab]);

    const handleCreate = async () => {
        if (!createForm.name.trim()) {
            Toast.warning('请输入名称');
            return;
        }
        try {
            const res = await API.post('/api/system-key/', createForm);
            const { success, data, message } = res.data;
            if (success) {
                Toast.success('创建成功');
                Modal.info({
                    title: '系统 Key 已创建',
                    content: (
                        <div>
                            <Banner type="warning" description="请妥善保管此 Key，关闭后将无法再次查看完整值！" style={{ marginBottom: 12 }} />
                            <Input value={data.key} readOnly addonAfter={
                                <Button icon={<IconCopy />} onClick={() => {
                                    navigator.clipboard.writeText(data.key);
                                    Toast.success('已复制');
                                }} />
                            } />
                        </div>
                    ),
                    okText: '我已保存',
                });
                setCreateVisible(false);
                setCreateForm({ name: '', expired_time: -1, remark: '' });
                loadKeys();
            } else {
                showError(message);
            }
        } catch (e) {
            showError(e.message);
        }
    };

    const handleUpdate = async () => {
        try {
            const res = await API.put('/api/system-key/' + editForm.id, editForm);
            const { success, message } = res.data;
            if (success) {
                Toast.success('更新成功');
                setEditVisible(false);
                loadKeys();
            } else {
                showError(message);
            }
        } catch (e) {
            showError(e.message);
        }
    };

    const handleDelete = async (id) => {
        try {
            const res = await API.delete('/api/system-key/' + id);
            const { success, message } = res.data;
            if (success) {
                Toast.success('删除成功');
                loadKeys();
            } else {
                showError(message);
            }
        } catch (e) {
            showError(e.message);
        }
    };

    const handleViewKey = async (id) => {
        try {
            const res = await API.get('/api/system-key/' + id + '/key');
            const { success, data, message } = res.data;
            if (success) {
                Modal.info({
                    title: '系统 Key 完整值',
                    content: (
                        <Input value={data} readOnly addonAfter={
                            <Button icon={<IconCopy />} onClick={() => {
                                navigator.clipboard.writeText(data);
                                Toast.success('已复制');
                            }} />
                        } />
                    ),
                });
            } else {
                showError(message);
            }
        } catch (e) {
            showError(e.message);
        }
    };

    const formatTimestamp = (ts) => {
        if (!ts || ts === -1) return '永不过期';
        return new Date(ts * 1000).toLocaleString();
    };

    const keyColumns = [
        { title: 'ID', dataIndex: 'id', width: 60 },
        { title: '名称', dataIndex: 'name', width: 150 },
        {
            title: 'Key',
            dataIndex: 'key',
            width: 220,
            render: (text) => (
                <Text copyable={{ content: text }} style={{ fontFamily: 'monospace', fontSize: 12 }}>
                    {text}
                </Text>
            ),
        },
        {
            title: '状态',
            dataIndex: 'status',
            width: 80,
            render: (status) => (
                <Tag color={status === 1 ? 'green' : 'red'} size="small">
                    {status === 1 ? '启用' : '禁用'}
                </Tag>
            ),
        },
        {
            title: '有效期',
            dataIndex: 'expired_time',
            width: 160,
            render: (ts) => {
                if (ts === -1) return <Tag color="blue" size="small">永不过期</Tag>;
                const now = Date.now() / 1000;
                const expired = ts < now;
                return (
                    <Tag color={expired ? 'red' : 'green'} size="small">
                        {expired ? '已过期 ' : ''}{formatTimestamp(ts)}
                    </Tag>
                );
            },
        },
        { title: '创建时间', dataIndex: 'created_time', width: 160, render: formatTimestamp },
        { title: '备注', dataIndex: 'remark', width: 150 },
        {
            title: '操作',
            width: 200,
            render: (_, record) => (
                <Space>
                    <Button size="small" icon={<IconEyeOpened />} onClick={() => handleViewKey(record.id)}>
                        查看
                    </Button>
                    <Button size="small" onClick={() => { setEditForm({ ...record }); setEditVisible(true); }}>
                        编辑
                    </Button>
                    <Popconfirm title="确定删除此系统 Key？" onConfirm={() => handleDelete(record.id)}>
                        <Button size="small" type="danger" icon={<IconDelete />}>
                            删除
                        </Button>
                    </Popconfirm>
                </Space>
            ),
        },
    ];

    const logColumns = [
        { title: 'ID', dataIndex: 'id', width: 60 },
        { title: 'Key 名称', dataIndex: 'key_name', width: 120 },
        {
            title: '操作',
            dataIndex: 'action',
            width: 120,
            render: (action) => {
                const colorMap = {
                    get_user_info: 'blue',
                    get_user_tokens: 'cyan',
                    get_user_models: 'teal',
                    create_token: 'green',
                    delete_token: 'red',
                };
                return <Tag color={colorMap[action] || 'grey'} size="small">{action}</Tag>;
            },
        },
        { title: '目标 OpenID', dataIndex: 'target_open_id', width: 180 },
        { title: '目标用户 ID', dataIndex: 'target_user_id', width: 100 },
        { title: 'IP', dataIndex: 'ip', width: 120 },
        {
            title: '状态',
            dataIndex: 'status',
            width: 70,
            render: (s) => <Tag color={s === 200 ? 'green' : 'red'} size="small">{s}</Tag>,
        },
        { title: '详情', dataIndex: 'detail', width: 200 },
        { title: '时间', dataIndex: 'created_at', width: 160, render: formatTimestamp },
    ];

    return (
        <div>
            <Banner
                type="info"
                description="系统 Key 用于授权第三方服务调用管理 API。第三方可通过 OpenID 查询用户信息、管理 API Key。请妥善保管系统 Key，避免泄露。"
                style={{ marginBottom: 16 }}
            />

            <Tabs activeKey={activeTab} onChange={setActiveTab}>
                <TabPane tab="Key 管理" itemKey="keys">
                    <div style={{ marginBottom: 16 }}>
                        <Button icon={<IconPlus />} theme="solid" onClick={() => setCreateVisible(true)}>
                            创建系统 Key
                        </Button>
                        <Button icon={<IconRefresh />} style={{ marginLeft: 8 }} onClick={loadKeys}>
                            刷新
                        </Button>
                    </div>

                    <Table
                        columns={keyColumns}
                        dataSource={keys}
                        loading={loading}
                        rowKey="id"
                        pagination={{
                            currentPage: page,
                            pageSize: pageSize,
                            total: total,
                            onPageChange: setPage,
                        }}
                    />
                </TabPane>

                <TabPane tab="调用日志" itemKey="logs">
                    <div style={{ marginBottom: 16, display: 'flex', gap: 8, alignItems: 'center' }}>
                        <Select
                            placeholder="筛选操作类型"
                            value={logFilter.action}
                            onChange={(v) => setLogFilter({ ...logFilter, action: v || '' })}
                            showClear
                            style={{ width: 180 }}
                            optionList={[
                                { label: '查询用户信息', value: 'get_user_info' },
                                { label: '查询用户令牌', value: 'get_user_tokens' },
                                { label: '查询可用模型', value: 'get_user_models' },
                                { label: '创建令牌', value: 'create_token' },
                                { label: '删除令牌', value: 'delete_token' },
                            ]}
                        />
                        <Button icon={<IconRefresh />} onClick={loadLogs}>
                            刷新
                        </Button>
                    </div>

                    <Table
                        columns={logColumns}
                        dataSource={logs}
                        loading={logLoading}
                        rowKey="id"
                        pagination={{
                            currentPage: logPage,
                            pageSize: 20,
                            total: logTotal,
                            onPageChange: setLogPage,
                        }}
                    />
                </TabPane>
            </Tabs>

            {/* 创建弹窗 */}
            <Modal
                title="创建系统 Key"
                visible={createVisible}
                onOk={handleCreate}
                onCancel={() => setCreateVisible(false)}
                okText="创建"
                cancelText="取消"
            >
                <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                    <div>
                        <div style={{ marginBottom: 4, fontWeight: 500 }}>名称 <span style={{ color: 'red' }}>*</span></div>
                        <Input
                            value={createForm.name}
                            onChange={(v) => setCreateForm({ ...createForm, name: v })}
                            placeholder="请输入系统 Key 名称"
                        />
                    </div>
                    <div>
                        <div style={{ marginBottom: 4, fontWeight: 500 }}>有效期</div>
                        <Select
                            value={createForm.expired_time}
                            onChange={(v) => setCreateForm({ ...createForm, expired_time: v })}
                            optionList={[
                                { label: '永不过期', value: -1 },
                                { label: '30 天', value: Math.floor(Date.now() / 1000) + 30 * 86400 },
                                { label: '90 天', value: Math.floor(Date.now() / 1000) + 90 * 86400 },
                                { label: '180 天', value: Math.floor(Date.now() / 1000) + 180 * 86400 },
                                { label: '365 天', value: Math.floor(Date.now() / 1000) + 365 * 86400 },
                            ]}
                            style={{ width: '100%' }}
                        />
                    </div>
                    <div>
                        <div style={{ marginBottom: 4, fontWeight: 500 }}>备注</div>
                        <Input
                            value={createForm.remark}
                            onChange={(v) => setCreateForm({ ...createForm, remark: v })}
                            placeholder="可选，用于记录用途"
                        />
                    </div>
                </div>
            </Modal>

            {/* 编辑弹窗 */}
            <Modal
                title="编辑系统 Key"
                visible={editVisible}
                onOk={handleUpdate}
                onCancel={() => setEditVisible(false)}
                okText="保存"
                cancelText="取消"
            >
                <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                    <div>
                        <div style={{ marginBottom: 4, fontWeight: 500 }}>名称</div>
                        <Input
                            value={editForm.name}
                            onChange={(v) => setEditForm({ ...editForm, name: v })}
                        />
                    </div>
                    <div>
                        <div style={{ marginBottom: 4, fontWeight: 500 }}>状态</div>
                        <Select
                            value={editForm.status}
                            onChange={(v) => setEditForm({ ...editForm, status: v })}
                            optionList={[
                                { label: '启用', value: 1 },
                                { label: '禁用', value: 2 },
                            ]}
                            style={{ width: '100%' }}
                        />
                    </div>
                    <div>
                        <div style={{ marginBottom: 4, fontWeight: 500 }}>有效期</div>
                        <Select
                            value={editForm.expired_time}
                            onChange={(v) => setEditForm({ ...editForm, expired_time: v })}
                            optionList={[
                                { label: '永不过期', value: -1 },
                                { label: '30 天后', value: Math.floor(Date.now() / 1000) + 30 * 86400 },
                                { label: '90 天后', value: Math.floor(Date.now() / 1000) + 90 * 86400 },
                                { label: '180 天后', value: Math.floor(Date.now() / 1000) + 180 * 86400 },
                                { label: '365 天后', value: Math.floor(Date.now() / 1000) + 365 * 86400 },
                            ]}
                            style={{ width: '100%' }}
                        />
                    </div>
                    <div>
                        <div style={{ marginBottom: 4, fontWeight: 500 }}>备注</div>
                        <Input
                            value={editForm.remark}
                            onChange={(v) => setEditForm({ ...editForm, remark: v })}
                        />
                    </div>
                </div>
            </Modal>
        </div>
    );
}

export default SystemKeySetting;
