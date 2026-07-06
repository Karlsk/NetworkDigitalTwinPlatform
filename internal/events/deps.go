// Package events 提供事件总线抽象接口与实现。
// 此文件保留 Kafka 客户端依赖，供后续 V2-02 Kafka Producer 实现使用。
package events

import _ "github.com/IBM/sarama" // V2-01: 预引入 Kafka 客户端，后续任务使用
