package dev.tavernagent.mobile.ui.screens.connect

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Lan
import androidx.compose.material.icons.filled.LockOpen
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import dev.tavernagent.mobile.ui.theme.dimens

/**
 * 连接与配对界面。地址支持 主机:端口 或完整 URL；HTTPS 自签证书走指纹确认（TOFU）。
 */
@Composable
fun ConnectScreen(onConnected: () -> Unit) {
    val viewModel: ConnectViewModel = viewModel(factory = ConnectViewModel.Factory)
    LaunchedEffect(Unit) { viewModel.onConnected = onConnected }
    val state by viewModel.state.collectAsStateWithLifecycle()
    val focus = LocalFocusManager.current

    Box(
        modifier = Modifier
            .fillMaxSize()
            .windowInsetsPadding(WindowInsets.safeDrawing)
            .imePadding(),
        contentAlignment = Alignment.TopCenter,
    ) {
        Column(
            modifier = Modifier
                .verticalScroll(rememberScrollState())
                .padding(horizontal = dimens.space5, vertical = dimens.space6)
                .widthIn(max = 480.dp),
            verticalArrangement = Arrangement.spacedBy(dimens.space4),
        ) {
            Spacer(Modifier.height(dimens.space5))
            Text(
                text = "TavernAgent",
                style = MaterialTheme.typography.headlineLarge,
                fontFamily = FontFamily.Serif,
            )
            Text(
                text = "连接到你的酒馆服务，在手机上继续和角色的对话。",
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )

            Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh)) {
                Column(
                    modifier = Modifier.padding(dimens.space4),
                    verticalArrangement = Arrangement.spacedBy(dimens.space3),
                ) {
                    OutlinedTextField(
                        value = state.address,
                        onValueChange = viewModel::onAddressChange,
                        modifier = Modifier.fillMaxWidth(),
                        label = { Text("服务地址") },
                        placeholder = { Text("192.168.1.5:8890") },
                        leadingIcon = { Icon(Icons.Filled.Lan, contentDescription = null) },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(
                            keyboardType = KeyboardType.Uri,
                            imeAction = if (state.pinRequired) ImeAction.Next else ImeAction.Done,
                        ),
                        keyboardActions = KeyboardActions(
                            onDone = {
                                focus.clearFocus()
                                viewModel.connect()
                            },
                        ),
                        enabled = !state.busy,
                    )

                    if (state.pinRequired) {
                        OutlinedTextField(
                            value = state.pin,
                            onValueChange = viewModel::onPinChange,
                            modifier = Modifier.fillMaxWidth(),
                            label = { Text("配对码") },
                            placeholder = { Text("服务端启动时显示的 6 位数字") },
                            leadingIcon = { Icon(Icons.Filled.LockOpen, contentDescription = null) },
                            singleLine = true,
                            visualTransformation = PasswordVisualTransformation(),
                            keyboardOptions = KeyboardOptions(
                                keyboardType = KeyboardType.NumberPassword,
                                imeAction = ImeAction.Done,
                            ),
                            keyboardActions = KeyboardActions(
                                onDone = {
                                    focus.clearFocus()
                                    viewModel.pair()
                                },
                            ),
                            enabled = !state.busy,
                        )
                    }

                    state.error?.let { message ->
                        Text(
                            text = message,
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.error,
                        )
                    }

                    Button(
                        onClick = { if (state.pinRequired) viewModel.pair() else viewModel.connect() },
                        modifier = Modifier.fillMaxWidth(),
                        enabled = !state.busy && state.address.isNotBlank(),
                    ) {
                        if (state.busy) {
                            CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp)
                            Spacer(Modifier.width(dimens.space2))
                        }
                        Text(if (state.pinRequired) "配对并进入" else "连接")
                    }
                }
            }

            state.trust?.let { trust ->
                TrustCard(
                    fingerprint = trust.fingerprint,
                    hostPort = trust.hostPort,
                    previousTrusted = trust.previousTrusted,
                    busy = state.busy,
                    onTrust = viewModel::trustCertificate,
                    onDismiss = viewModel::dismissTrust,
                )
            }

            Text(
                text = "提示：电脑上运行 `go run ./cmd/tavernagent` 后，手机与电脑需处于同一局域网。" +
                    "服务开启 -tls auto 时请使用 https:// 地址。",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun TrustCard(
    fingerprint: String,
    hostPort: String,
    previousTrusted: Boolean,
    busy: Boolean,
    onTrust: () -> Unit,
    onDismiss: () -> Unit,
) {
    Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.errorContainer)) {
        Column(
            modifier = Modifier.padding(dimens.space4),
            verticalArrangement = Arrangement.spacedBy(dimens.space2),
        ) {
            Row(
                horizontalArrangement = Arrangement.spacedBy(dimens.space2),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Icon(Icons.Filled.Warning, contentDescription = null, tint = MaterialTheme.colorScheme.onErrorContainer)
                Text(
                    text = if (previousTrusted) "服务器证书已变更" else "首次连接：需要确认服务器证书",
                    style = MaterialTheme.typography.titleSmall,
                    color = MaterialTheme.colorScheme.onErrorContainer,
                )
            }
            Text(
                text = "$hostPort\nSHA-256 $fingerprint",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onErrorContainer,
            )
            Text(
                text = "如果确认这是自己电脑上的服务（-tls auto 生成自签证书），可以信任该指纹。" +
                    "证书变更可能意味着中间人攻击，请先核对。",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onErrorContainer,
            )
            Row(horizontalArrangement = Arrangement.spacedBy(dimens.space2)) {
                Button(onClick = onTrust, enabled = !busy) { Text("信任并继续") }
                TextButton(onClick = onDismiss, enabled = !busy) { Text("取消") }
            }
        }
    }
}
